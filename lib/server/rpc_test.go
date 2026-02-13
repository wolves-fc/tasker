package server

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/wolves-fc/tasker/lib/rpc"
	"github.com/wolves-fc/tasker/lib/tls"
)

func TestCheckJobAccess(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		id    rpc.Identity
		owner string
		want  codes.Code
	}{
		{"admin_own_job", rpc.Identity{Name: "wolf", Role: tls.RoleAdmin}, "wolf", codes.OK},
		{"admin_other_job", rpc.Identity{Name: "wolf", Role: tls.RoleAdmin}, "wolfjr", codes.OK},
		{"user_own_job", rpc.Identity{Name: "wolfjr", Role: tls.RoleUser}, "wolfjr", codes.OK},
		{"user_other_job", rpc.Identity{Name: "wolfjr", Role: tls.RoleUser}, "wolf", codes.PermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkJobAccess(tc.id, tc.owner)
			got := status.Code(err)

			if got != tc.want {
				t.Errorf("code (got=%v, want=%v)", got, tc.want)
			}
		})
	}
}
