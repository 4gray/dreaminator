package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"dreaminator/internal/enigma"
)

func (a *App) receiverStore() enigma.ReceiverStore {
	return enigma.ReceiverStore{Dir: a.cfg.E2Dir, BackupDir: filepath.Join(a.cfg.DataDir, "receiver-undo")}
}
func (a *App) ReceiverBouquets() ([]enigma.ReceiverBouquet, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	return a.receiverStore().List()
}
func (a *App) ReceiverBouquet(ctx context.Context, id string) (enigma.ReceiverBouquet, error) {
	a.applyMu.Lock()
	b, err := a.receiverStore().Read(id)
	a.applyMu.Unlock()
	if err != nil {
		return b, err
	}
	needsNames := false
	for _, c := range b.Channels {
		if strings.HasPrefix(c.Name, "Канал ") {
			needsNames = true
			break
		}
	}
	if !needsNames {
		return b, nil
	}
	a.mu.Lock()
	settings := a.st.Settings
	a.mu.Unlock()
	services, err := a.webif(settings).Services(ctx, id)
	if err == nil {
		names := map[string]string{}
		for _, svc := range services {
			names[svc.Reference] = svc.Name
		}
		for i, c := range b.Channels {
			if name := names[c.Reference]; name != "" && strings.HasPrefix(c.Name, "Канал ") {
				b.Channels[i].Name = name
			}
		}
	}
	return b, nil
}

type ReceiverResult struct {
	Bouquet enigma.ReceiverBouquet `json:"bouquet"`
	Warning string                 `json:"warning,omitempty"`
}

func (a *App) ReloadReceiverBouquets(ctx context.Context) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	return a.reloadReceiver(ctx)
}
func (a *App) reloadReceiver(ctx context.Context) error {
	a.mu.Lock()
	settings := a.st.Settings
	a.mu.Unlock()
	return a.webif(settings).ReloadBouquets(ctx)
}
func (a *App) receiverResult(ctx context.Context, b enigma.ReceiverBouquet, err error) (ReceiverResult, error) {
	if err != nil {
		return ReceiverResult{}, err
	}
	result := ReceiverResult{Bouquet: b}
	if err := a.reloadReceiver(ctx); err != nil {
		result.Warning = "Файл сохранён, но ресивер не перечитал списки: " + err.Error()
	}
	return result, nil
}
func (a *App) SaveReceiverBouquet(ctx context.Context, id string, p enigma.ReceiverPatch) (ReceiverResult, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	b, err := a.receiverStore().Save(id, p)
	return a.receiverResult(ctx, b, err)
}
func (a *App) UndoReceiverBouquet(ctx context.Context, id, revision string) (ReceiverResult, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	b, err := a.receiverStore().Undo(id, revision)
	return a.receiverResult(ctx, b, err)
}
func (a *App) CopyReceiverBouquet(ctx context.Context, id, revision, name string) (ReceiverResult, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	b, err := a.receiverStore().Copy(id, revision, name)
	return a.receiverResult(ctx, b, err)
}
func (a *App) RenameReceiverBouquet(ctx context.Context, id, revision, name string) (ReceiverResult, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	b, err := a.receiverStore().Rename(id, revision, name)
	return a.receiverResult(ctx, b, err)
}

// DeleteReceiverBouquet returns a warning when the bouquet is unregistered but
// its file stayed behind or Enigma2 did not re-read its lists.
func (a *App) DeleteReceiverBouquet(ctx context.Context, id, revision string) (string, error) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	var warning string
	err := a.receiverStore().Delete(id, revision)
	if errors.Is(err, enigma.ErrReceiverFileLeft) {
		// Unregistered already: Enigma2 must still re-read its lists.
		warning, err = err.Error()+". ", nil
	}
	if err != nil {
		return "", err
	}
	if err := a.reloadReceiver(ctx); err != nil {
		warning += "Букет удалён, но ресивер не перечитал списки: " + err.Error()
	}
	return warning, nil
}
