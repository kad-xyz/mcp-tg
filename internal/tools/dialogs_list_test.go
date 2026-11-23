package tools

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/lexfrei/mcp-tg/internal/telegram"
)

func TestDialogsListTool_Definition(t *testing.T) {
	tool := DialogsListTool()
	if tool.Name == "" {
		t.Error("tool name must not be empty")
	}
}

func TestDialogsListHandler_Success(t *testing.T) {
	mock := &mockClient{
		dialogs: []telegram.Dialog{
			{Title: "Chat 1", IsGroup: true},
			{Title: "Chat 2", IsChannel: true},
		},
	}
	handler := NewDialogsListHandler(mock)

	_, structured, err := handler(context.Background(), nil, DialogsListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Count != 2 {
		t.Errorf("Count = %d, want 2", structured.Count)
	}
}

func TestDialogsListHandler_Error(t *testing.T) {
	mock := &mockClient{err: errors.New("fail")}
	handler := NewDialogsListHandler(mock)

	result, _, err := handler(context.Background(), nil, DialogsListParams{})
	if err == nil {
		t.Fatal("expected error")
	}

	if result == nil || !result.IsError {
		t.Error("result.IsError should be true")
	}
}

// limit sizes the wrapper's collections, so an unbounded one could crash the
// process before the first request. Both tools that page dialogs cap it.
func TestDialogsListHandlers_RejectALimitAboveTheCap(t *testing.T) {
	limit := maxDialogsLimit + 1

	_, _, err := NewDialogsListHandler(&mockClient{})(context.Background(), nil, DialogsListParams{Limit: &limit})
	if !errors.Is(err, ErrLimitTooLarge) {
		t.Errorf("tg_dialogs_list: err = %v, want ErrLimitTooLarge", err)
	}

	_, _, err = NewGroupsListHandler(&mockClient{})(context.Background(), nil, GroupsListParams{Limit: &limit})
	if !errors.Is(err, ErrLimitTooLarge) {
		t.Errorf("tg_groups_list: err = %v, want ErrLimitTooLarge", err)
	}
}

func TestDialogsListHandler_AcceptsTheCap(t *testing.T) {
	limit := maxDialogsLimit

	_, _, err := NewDialogsListHandler(&mockClient{})(context.Background(), nil, DialogsListParams{Limit: &limit})
	if err != nil {
		t.Errorf("limit %d: unexpected error %v", limit, err)
	}
}

// lastMessageDate is the cursor a caller pages with, so it must reach the item.
func TestDialogsListHandler_ReportsLastMessageDate(t *testing.T) {
	mock := &mockClient{dialogs: []telegram.Dialog{{Title: "Chat", LastMessageDate: 1700000000}}}

	_, structured, err := NewDialogsListHandler(mock)(context.Background(), nil, DialogsListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := structured.Dialogs[0].LastMessageDate; got != 1700000000 {
		t.Errorf("LastMessageDate = %d, want 1700000000", got)
	}
}

func TestDialogsListHandler_PassesArchived(t *testing.T) {
	mock := &mockClient{}
	handler := NewDialogsListHandler(mock)
	archived := true

	_, _, err := handler(context.Background(), nil, DialogsListParams{Archived: &archived})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.lastDialogOpts.Archived {
		t.Error("archived: true did not reach the client")
	}
}

// pagingDialogsClient serves a long dialog list a page at a time, older than
// OffsetDate, the way the wrapper does.
type pagingDialogsClient struct {
	*mockClient

	all []telegram.Dialog
}

func (p *pagingDialogsClient) GetDialogs(_ context.Context, opts telegram.DialogOpts) ([]telegram.Dialog, error) {
	var page []telegram.Dialog

	for _, dialog := range p.all {
		if len(page) == min(opts.Limit, 100) {
			break
		}

		if opts.OffsetDate == 0 || dialog.LastMessage.Date < opts.OffsetDate {
			page = append(page, dialog)
		}
	}

	return page, nil
}

// With TELEGRAM_ALLOWED_PEERS set, an allowed chat beyond the first source
// page is listed, and hasMore does not claim the list ended before it.
func TestDialogsListHandler_AllowlistReachesPastTheFirstPage(t *testing.T) {
	all := make([]telegram.Dialog, 150)
	for idx := range all {
		all[idx] = telegram.Dialog{
			Peer:        telegram.InputPeer{Type: telegram.PeerUser, ID: int64(1000 + idx)},
			Title:       "chat",
			LastMessage: &telegram.Message{Date: 100000 - idx},
		}
	}

	client := telegram.NewPeerAllowlist(&pagingDialogsClient{mockClient: &mockClient{}, all: all}, []string{"1149"})

	_, structured, err := NewDialogsListHandler(client)(context.Background(), nil, DialogsListParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if structured.Count != 1 || structured.Dialogs[0].Peer != "1149" || structured.HasMore {
		t.Errorf("count = %d, hasMore = %v, dialogs = %+v; want the one allowed chat and no more",
			structured.Count, structured.HasMore, structured.Dialogs)
	}
}
