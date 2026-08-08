package updater

import (
	"strings"
	"testing"
)

func TestRuntimeURLsUseDualSenseClientRepository(t *testing.T) {
	t.Parallel()

	urls := map[string]string{
		"repository": repositoryURL,
		"releases":   releasesAPIURL,
		"installer":  installScriptBaseURL,
		"release":    releaseURL("v1.2.3"),
	}
	for name, runtimeURL := range urls {
		if !strings.Contains(runtimeURL, "DualSenseClient/VIIPER") {
			t.Errorf("%s URL %q does not target DualSenseClient/VIIPER", name, runtimeURL)
		}
		if strings.Contains(strings.ToLower(runtimeURL), "alia5") {
			t.Errorf("%s URL %q retains the Alia5 repository", name, runtimeURL)
		}
	}
}

func TestReleaseURLEscapesTag(t *testing.T) {
	t.Parallel()

	const want = "https://github.com/DualSenseClient/VIIPER/releases/tag/release%2Fcandidate"
	if got := releaseURL("release/candidate"); got != want {
		t.Fatalf("releaseURL() = %q, want %q", got, want)
	}
}
