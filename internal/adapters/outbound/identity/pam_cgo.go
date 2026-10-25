//go:build cgo

package identity

/*
#cgo LDFLAGS: -lpam -lcrypt
#define _GNU_SOURCE
#include <crypt.h>
#include <security/pam_appl.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    const char *username;
    const char *password;
} pam_creds_t;

static int pam_conv_callback(int num_msg, const struct pam_message **msg,
                             struct pam_response **resp, void *appdata_ptr) {
    if (num_msg <= 0) {
        return PAM_CONV_ERR;
    }
    struct pam_response *reply = (struct pam_response *)calloc(num_msg, sizeof(struct pam_response));
    if (!reply) {
        return PAM_BUF_ERR;
    }
    pam_creds_t *creds = (pam_creds_t *)appdata_ptr;
    for (int i = 0; i < num_msg; i++) {
        int style = msg[i]->msg_style;
        if (style == PAM_PROMPT_ECHO_OFF) {
            reply[i].resp = strdup(creds && creds->password ? creds->password : "");
        } else if (style == PAM_PROMPT_ECHO_ON) {
            reply[i].resp = strdup(creds && creds->username ? creds->username : "");
        } else {
            reply[i].resp = strdup("");
        }
        reply[i].resp_retcode = 0;
    }
    *resp = reply;
    return PAM_SUCCESS;
}

int authenticate_via_pam(const char *service, const char *username, const char *password) {
    pam_handle_t *pamh = NULL;
    pam_creds_t creds = {username, password};
    struct pam_conv conv = {pam_conv_callback, (void *)&creds};

    int ret = pam_start(service, username, &conv, &pamh);
    if (ret != PAM_SUCCESS) {
        return ret;
    }

    // Set PAM items so non-interactive modules succeed
    pam_set_item(pamh, PAM_AUTHTOK, (const void *)password);
    pam_set_item(pamh, PAM_TTY, (const void *)"nasctl");
    pam_set_item(pamh, PAM_RUSER, (const void *)username);

    ret = pam_authenticate(pamh, PAM_SILENT | PAM_DISALLOW_NULL_AUTHTOK);
    if (ret == PAM_SUCCESS) {
        int acct_ret = pam_acct_mgmt(pamh, PAM_SILENT);
        if (acct_ret != PAM_SUCCESS) {
            ret = acct_ret;
        }
    }

    pam_end(pamh, ret);
    return ret;
}

int verify_shadow_crypt(const char *password, const char *hash) {
    if (!password || !hash || hash[0] == '*' || hash[0] == '!' || hash[0] == '\0') {
        return 0;
    }
    struct crypt_data data;
    memset(&data, 0, sizeof(data));
    char *result = crypt_r(password, hash, &data);
    if (!result) {
        return 0;
    }
    return (strcmp(result, hash) == 0) ? 1 : 0;
}
*/
import "C"
import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

// AuthenticateShadow checks the candidate password directly against /etc/shadow using crypt_r.
// Returns (true, nil) if verified, (false, nil) if invalid, or error if shadow cannot be read.
func AuthenticateShadow(username, password string) (bool, error) {
	f, err := os.Open("/etc/shadow")
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && parts[0] == username {
			storedHash := parts[1]
			cPw := C.CString(password)
			cHash := C.CString(storedHash)
			defer C.free(unsafe.Pointer(cPw))
			defer C.free(unsafe.Pointer(cHash))

			res := C.verify_shadow_crypt(cPw, cHash)
			return res == 1, nil
		}
	}
	return false, fmt.Errorf("user %q not found in /etc/shadow", username)
}

// AuthenticatePAM attempts to authenticate a username and password against Linux PAM.
func AuthenticatePAM(serviceName, username, password string) error {
	if serviceName == "" {
		if _, err := os.Stat("/etc/pam.d/system-auth"); err == nil {
			serviceName = "system-auth"
		} else {
			serviceName = "login"
		}
	}

	cService := C.CString(serviceName)
	cUser := C.CString(username)
	cPw := C.CString(password)
	defer C.free(unsafe.Pointer(cService))
	defer C.free(unsafe.Pointer(cUser))
	defer C.free(unsafe.Pointer(cPw))

	ret := C.authenticate_via_pam(cService, cUser, cPw)
	if ret != C.PAM_SUCCESS {
		return fmt.Errorf("PAM error (%s): code %d", serviceName, int(ret))
	}
	return nil
}
