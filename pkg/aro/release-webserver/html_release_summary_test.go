package release_webserver

import (
	"testing"

	"github.com/openshift-online/service-status/pkg/aro/client"
)

func Test_htmlReleaseSummary_ServeGin(t *testing.T) {
	type fields struct {
		releaseClient client.ReleaseClient
	}
	type args struct {
		c *gin.Context
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &htmlReleaseSummary{
				releaseClient: tt.fields.releaseClient,
			}
			h.ServeGin(tt.args.c)
		})
	}
}