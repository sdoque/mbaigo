package usecases

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sdoque/mbaigo/components"
)

// A service that offers no stream must refuse to stream, not panic.
//
// Found on a running cloud. kgrapher asked to follow the registrar's syslist,
// which is not subscribable and so has a nil Stream; the guard called
// Subscribable() on the nil interface, net/http recovered the panic and closed
// the connection without a status line, and kgrapher logged "EOF; reconnecting
// shortly" every fourteen seconds. It never rebuilt, so it never published to
// the triple store — a graph that had been empty for as long as anyone had been
// relying on it, with no error anywhere that named the cause.
func TestAskingToFollowAnUnfollowableServiceDoesNotPanic(t *testing.T) {
	sys := components.NewSystem("serviceregistrar", context.Background())
	sys.Husk = &components.Husk{}
	served := false
	sys.UAssets["registry"] = &components.UnitAsset{
		Name:    "registry",
		Mission: components.MissionCore,
		ServicesMap: components.Services{
			// Stream left nil, as it is for every service the framework has not
			// prepared one for — which is most of them.
			"syslist": {Definition: "syslist", SubPath: "syslist"},
		},
		ServingFunc: func(w http.ResponseWriter, r *http.Request, servicePath string) {
			served = true
			w.WriteHeader(http.StatusOK)
		},
	}

	r := httptest.NewRequest(http.MethodGet, "/serviceregistrar/registry/syslist", nil)
	r.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("asking to follow an unfollowable service panicked: %v", p)
		}
	}()
	handleFourParts(w, r, "registry", "syslist", &sys)

	// And it reaches the unit asset, which is what lets a system implement its
	// own streaming — the ESR's registry subscription is exactly that.
	if !served {
		t.Error("the request never reached the unit asset, so a system cannot serve its own stream")
	}
}
