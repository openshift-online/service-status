package release_webserver

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openshift-online/service-status/pkg/aro/client"
	"github.com/stretchr/testify/assert"
)

//go:embed test-artifacts
var testArtifacts embed.FS

func TestReleaseSummaryHTML(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "basic",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFS, err := fs.Sub(testArtifacts, filepath.Join("test-artifacts/ReleaseSummaryHTML/", tt.name))
			assert.NoError(t, err)
			basicClient := client.NewFileSystemReleaseClient(testFS)

			httpRouter := gin.Default()
			httpRouter.LoadHTMLGlob("/home/deads/workspaces/service-status/src/gitub.com/openshift-online/service-status/pkg/aro/release-webserver/html-templates/*")
			httpRouter.GET("/http/aro-hcp/summary.html", ServeReleaseSummary(basicClient))

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/http/aro-hcp/summary.html", nil)
			httpRouter.ServeHTTP(w, req)

			expectedHTML, err := testArtifacts.ReadFile(filepath.Join("test-artifacts/ReleaseSummaryHTML", tt.name, "expected-summary.html"))
			assert.NoError(t, err)
			assert.Equal(t, 200, w.Code)
			t.Log(w.Body.String())
			assert.Equal(t, string(expectedHTML), w.Body.String())
		})
	}
}
