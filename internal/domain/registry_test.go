package domain

import "testing"

func TestRegistryForImage(t *testing.T) {
	cases := map[string]string{
		"nginx":                               "docker.io",
		"library/nginx:1":                     "docker.io",
		"jc21/nginx-proxy-manager:latest":     "docker.io",
		"docker.io/library/postgres@sha256:x": "docker.io",
		"ghcr.io/sigxcpu76/quasar:0.1.23":     "ghcr.io",
		"registry.example.com:5000/a/b":       "registry.example.com:5000",
		"localhost/foo":                       "localhost",
	}
	for img, want := range cases {
		if got := RegistryForImage(img); got != want {
			t.Errorf("RegistryForImage(%q)=%q want %q", img, got, want)
		}
	}
}

func TestNormalizeRegistryServer(t *testing.T) {
	for in, want := range map[string]string{
		"":                            "docker.io",
		"https://index.docker.io/v1/": "docker.io",
		"registry-1.docker.io":        "docker.io",
		"https://GHCR.io/":            "ghcr.io",
		"registry.example.com:5000":   "registry.example.com:5000",
	} {
		if got := NormalizeRegistryServer(in); got != want {
			t.Errorf("NormalizeRegistryServer(%q)=%q want %q", in, got, want)
		}
	}
	if RegistryAuthKey("docker.io") != DockerHubAuthKey || RegistryAuthKey("ghcr.io") != "ghcr.io" {
		t.Error("bad RegistryAuthKey")
	}
}
