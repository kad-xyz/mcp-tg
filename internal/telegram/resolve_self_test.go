package telegram

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
)

const (
	resolveSelfID   = 4242
	resolveSelfHash = 4343
)

// selfInvoker answers users.getUsers(inputUserSelf) and nothing else, so a
// self alias that falls through to contacts.resolveUsername fails the test.
type selfInvoker struct {
	calls atomic.Int32
}

func (s *selfInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	s.calls.Add(1)

	req, ok := input.(*tg.UsersGetUsersRequest)
	if !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	if len(req.ID) != 1 {
		return errors.Wrapf(errUnexpectedRequest, "users.getUsers with %d ids", len(req.ID))
	}

	if _, self := req.ID[0].(*tg.InputUserSelf); !self {
		return errors.Wrapf(errUnexpectedRequest, "users.getUsers(%T)", req.ID[0])
	}

	return encodeResp(&tg.UserClassVector{Elems: []tg.UserClass{
		&tg.User{ID: resolveSelfID, AccessHash: resolveSelfHash, Self: true},
	}}, output)
}

func TestResolve_SelfAliasesResolveToTheAccount(t *testing.T) {
	for _, alias := range []string{"me", "self", "@me", "ME", " Self "} {
		t.Run(alias, func(t *testing.T) {
			invoker := &selfInvoker{}

			got, err := Resolve(t.Context(), tg.NewClient(invoker), alias)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", alias, err)
			}

			want := InputPeer{Type: PeerUser, ID: resolveSelfID, AccessHash: resolveSelfHash}
			if got != want {
				t.Errorf("Resolve(%q) = %+v, want %+v", alias, got, want)
			}

			if n := invoker.calls.Load(); n != 1 {
				t.Errorf("Resolve(%q) made %d requests, want 1", alias, n)
			}
		})
	}
}

func TestResolve_SelfAliasWithoutSelfUserFails(t *testing.T) {
	invoker := &emptyUsersInvoker{}

	_, err := Resolve(t.Context(), tg.NewClient(invoker), "me")
	if !errors.Is(err, ErrPeerNotFound) {
		t.Errorf("err = %v, want ErrPeerNotFound", err)
	}
}

// emptyUsersInvoker answers users.getUsers with no users at all.
type emptyUsersInvoker struct{}

func (emptyUsersInvoker) Invoke(_ context.Context, input bin.Encoder, output bin.Decoder) error {
	if _, ok := input.(*tg.UsersGetUsersRequest); !ok {
		return errors.Wrapf(errUnexpectedRequest, "%T", input)
	}

	return encodeResp(&tg.UserClassVector{}, output)
}
