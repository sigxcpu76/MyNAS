//go:build cgo

package identity

/*
#cgo LDFLAGS: -lpam
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

*/
import "C"
import (
	"fmt"
	"os"
	"unsafe"
)

// PAMSupported reports whether this build can authenticate logins via PAM.
const PAMSupported = true

// AuthenticatePAM attempts to authenticate a username and password against Linux PAM.
func AuthenticatePAM(serviceName, username, password string) error {
	if serviceName == "" {
		serviceName = "login"
		// Prefer a dedicated /etc/pam.d/nasctl policy when the admin installed one.
		for _, candidate := range []string{"nasctl", "system-auth"} {
			if _, err := os.Stat("/etc/pam.d/" + candidate); err == nil {
				serviceName = candidate
				break
			}
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
