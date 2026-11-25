package ops

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go-atomcode-switch-cli/internal/atomio"
	"go-atomcode-switch-cli/internal/models"
	"go-atomcode-switch-cli/internal/store"

	"gopkg.in/yaml.v3"
)

func makeAccount(id, name, username, token string, status models.InfoStatus, usage float32) models.ManagedAccount {
	return models.ManagedAccount{
		ID: id,
		AuthData: models.AuthToml{
			AccessToken: token,
			User: models.User{
				ID:       id,
				Name:     name,
				Username: username,
			},
		},
		PlanName:     "未知套餐",
		UsagePercent: usage,
		ResetTime:    "2099-01-01 00:00",
		IsValid:      true,
		InfoStatus:   status,
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	atomDir := filepath.Join(root, "atomcode")
	cfg := models.DefaultConfig()
	cfg.SetAtomcodeDir(atomDir)
	st := store.New(dataDir)
	if err := st.Save(cfg); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	return New(cfg, st)
}

func TestResolveByIndexIDNameAndUsername(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("id-a", "Alpha", "alpha-user", "ta", models.StatusFresh, 1),
		makeAccount("id-b", "Beta", "beta-user", "tb", models.StatusFresh, 2),
	}
	for _, pattern := range []string{"2", "id-b", "Beta", "beta-user"} {
		acc, err := svc.Resolve(pattern)
		if err != nil || acc.ID != "id-b" {
			t.Fatalf("Resolve(%q) = %+v, %v", pattern, acc, err)
		}
	}
	if _, err := svc.Resolve("9"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Resolve(9) error = %v, want ErrNotFound", err)
	}
}

func TestAutoSwitchUsesHysteresisAndFreshOnly(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.AutoSwitchRules = models.AutoSwitchRules{Enabled: true, MaxUsagePercent: 95}
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("a", "A", "a", "ta", models.StatusFresh, 96),
		makeAccount("near", "Near", "near", "tn", models.StatusFresh, 92),
		makeAccount("unknown", "Unknown", "unknown", "tu", models.StatusUnknown, 10),
		makeAccount("best", "Best", "best", "tb", models.StatusFresh, 90),
	}
	svc.Cfg.SetActiveID("a")

	name, switched := svc.CheckAutoSwitch()
	if !switched || name != "Best" {
		t.Fatalf("CheckAutoSwitch() = %q, %v, want Best,true", name, switched)
	}
	if svc.Cfg.ActiveID() != "best" {
		t.Fatalf("active = %q, want best", svc.Cfg.ActiveID())
	}
	auth, err := atomio.Read(svc.Cfg.AtomcodeDir())
	if err != nil || auth.AccessToken != "tb" {
		t.Fatalf("auth token = %+v, err = %v", auth, err)
	}
}

func TestAutoSwitchBoundary(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.AutoSwitchRules = models.AutoSwitchRules{Enabled: true, MaxUsagePercent: 95}
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("a", "A", "a", "ta", models.StatusFresh, 96),
		makeAccount("b", "B", "b", "tb", models.StatusFresh, 91),
	}
	svc.Cfg.SetActiveID("a")
	if _, switched := svc.CheckAutoSwitch(); !switched {
		t.Fatal("exactly 5% lower account should be selected")
	}

	svc = newTestService(t)
	svc.Cfg.AutoSwitchRules = models.AutoSwitchRules{Enabled: true, MaxUsagePercent: 95}
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("a", "A", "a", "ta", models.StatusFresh, 96),
		makeAccount("b", "B", "b", "tb", models.StatusFresh, 91.5),
	}
	svc.Cfg.SetActiveID("a")
	if _, switched := svc.CheckAutoSwitch(); switched {
		t.Fatal("less than 5% lower account must not be selected")
	}
}

func TestApplyLoginOutputRejectsUnmatchedLoginUser(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("a", "A", "a", "ta", models.StatusFresh, 0),
	}
	svc.Cfg.SetActiveID("a")
	_, err := svc.ApplyLoginOutput("logged in as B\nUsage: 12%")
	if !errors.Is(err, ErrUserUnmatched) {
		t.Fatalf("error = %v, want ErrUserUnmatched", err)
	}
	if svc.Cfg.Accounts[0].UsagePercent != 0 || svc.Cfg.Accounts[0].InfoStatus != models.StatusFresh {
		t.Fatalf("account was modified: %+v", svc.Cfg.Accounts[0])
	}
}

func TestApplyLoginOutputSwitchesTargetAndWritesAuth(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("a", "A", "a", "ta", models.StatusFresh, 0),
		makeAccount("b", "B", "b", "tb", models.StatusFresh, 0),
	}
	svc.Cfg.SetActiveID("a")
	text := "logged in as B\nPlan: Pro Plan\nUsage: 12%\nresets 21:04"
	name, err := svc.ApplyLoginOutput(text)
	if err != nil || name != "B" {
		t.Fatalf("ApplyLoginOutput() = %q, %v", name, err)
	}
	if svc.Cfg.ActiveID() != "b" {
		t.Fatalf("active = %q, want b", svc.Cfg.ActiveID())
	}
	if svc.Cfg.Accounts[0].UsagePercent != 0 {
		t.Fatalf("active account was modified: %+v", svc.Cfg.Accounts[0])
	}
	target := svc.Cfg.Accounts[1]
	if target.PlanName != "Pro Plan" || target.UsagePercent != 12 || target.InfoStatus != models.StatusFresh || target.ResetTime == "" {
		t.Fatalf("target account not updated: %+v", target)
	}
	auth, err := atomio.Read(svc.Cfg.AtomcodeDir())
	if err != nil || auth.AccessToken != "tb" {
		t.Fatalf("target auth not written: %+v, %v", auth, err)
	}
	reloaded, err := svc.Store.Load()
	if err != nil || reloaded.ActiveID() != "b" {
		t.Fatalf("active not persisted: %+v, %v", reloaded, err)
	}
}

func TestImportCurrentAuthNewAndExisting(t *testing.T) {
	svc := newTestService(t)
	auth := &models.AuthToml{
		AccessToken: "t1",
		User:        models.User{ID: "u1", Name: "User One", Username: "u1"},
	}
	if err := atomio.Write(svc.Cfg.AtomcodeDir(), auth); err != nil {
		t.Fatal(err)
	}
	isNew, name, err := svc.ImportCurrentAuth()
	if err != nil || !isNew || name != "User One" {
		t.Fatalf("first import = %v, %q, %v", isNew, name, err)
	}
	if len(svc.Cfg.Accounts) != 1 || svc.Cfg.ActiveID() != "u1" {
		t.Fatalf("unexpected config: %+v", svc.Cfg)
	}

	auth.AccessToken = "t2"
	if err := atomio.Write(svc.Cfg.AtomcodeDir(), auth); err != nil {
		t.Fatal(err)
	}
	isNew, _, err = svc.ImportCurrentAuth()
	if err != nil || isNew {
		t.Fatalf("second import = %v, %v", isNew, err)
	}
	if len(svc.Cfg.Accounts) != 1 || svc.Cfg.Accounts[0].AuthData.AccessToken != "t2" {
		t.Fatalf("existing account not refreshed: %+v", svc.Cfg.Accounts)
	}
}

func TestSyncCurrentAuthExistingDoesNotAddAccount(t *testing.T) {
	svc := newTestService(t)
	unknown := &models.AuthToml{User: models.User{ID: "unknown", Name: "Unknown"}}
	if err := atomio.Write(svc.Cfg.AtomcodeDir(), unknown); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncCurrentAuthExisting(); err != nil {
		t.Fatal(err)
	}
	if len(svc.Cfg.Accounts) != 0 || svc.Cfg.ActiveID() != "" {
		t.Fatalf("unknown auth added an account: %+v", svc.Cfg)
	}

	svc.Cfg.Accounts = []models.ManagedAccount{makeAccount("known", "Known", "known", "old", models.StatusUnknown, 0)}
	known := &models.AuthToml{AccessToken: "fresh", User: models.User{ID: "known", Name: "Known"}}
	if err := atomio.Write(svc.Cfg.AtomcodeDir(), known); err != nil {
		t.Fatal(err)
	}
	if err := svc.SyncCurrentAuthExisting(); err != nil {
		t.Fatal(err)
	}
	if len(svc.Cfg.Accounts) != 1 || svc.Cfg.Accounts[0].AuthData.AccessToken != "fresh" || svc.Cfg.ActiveID() != "known" {
		t.Fatalf("existing account not synced: %+v", svc.Cfg)
	}
}

func TestDeleteActiveAccountSwitchesToFirstRemaining(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("a", "A", "a", "ta", models.StatusFresh, 0),
		makeAccount("b", "B", "b", "tb", models.StatusFresh, 0),
	}
	svc.Cfg.SetActiveID("a")
	if err := svc.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if len(svc.Cfg.Accounts) != 1 || svc.Cfg.ActiveID() != "b" {
		t.Fatalf("unexpected config after delete: %+v", svc.Cfg)
	}
	auth, err := atomio.Read(svc.Cfg.AtomcodeDir())
	if err != nil || auth.AccessToken != "tb" {
		t.Fatalf("replacement auth not written: %+v, %v", auth, err)
	}
}

func TestImportFileIsTransactionalAndPreservesLocalSettings(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.Language = "zh_cn"
	svc.Cfg.SortMode = models.SortNone
	svc.Cfg.Accounts = []models.ManagedAccount{
		makeAccount("a", "A", "a", "ta", models.StatusFresh, 1),
	}
	if err := svc.Save(); err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(svc.DataDir)

	badPath := filepath.Join(root, "bad.yaml")
	if err := os.WriteFile(badPath, []byte("accounts: [bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ImportFile(badPath); err == nil {
		t.Fatal("bad import unexpectedly succeeded")
	}
	if len(svc.Cfg.Accounts) != 1 || svc.Cfg.Accounts[0].ID != "a" {
		t.Fatalf("bad import changed accounts: %+v", svc.Cfg.Accounts)
	}

	importedCfg := models.DefaultConfig()
	importedCfg.Accounts = []models.ManagedAccount{
		makeAccount("c", "C", "c", "tc", models.StatusFresh, 2),
	}
	importedCfg.SetActiveID("c")
	importedCfg.AutoSwitchRules = models.AutoSwitchRules{Enabled: true, MaxUsagePercent: 70}
	importedCfg.Language = "en_us"
	importedCfg.SortMode = models.SortResetTime
	data, err := yaml.Marshal(importedCfg)
	if err != nil {
		t.Fatal(err)
	}
	goodPath := filepath.Join(root, "good.yaml")
	if err := os.WriteFile(goodPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := svc.ImportFile(goodPath)
	if err != nil || n != 1 {
		t.Fatalf("ImportFile() = %d, %v", n, err)
	}
	if len(svc.Cfg.Accounts) != 1 || svc.Cfg.Accounts[0].ID != "c" || svc.Cfg.ActiveID() != "c" {
		t.Fatalf("accounts not imported: %+v", svc.Cfg)
	}
	if !svc.Cfg.AutoSwitchRules.Enabled || svc.Cfg.AutoSwitchRules.MaxUsagePercent != 70 {
		t.Fatalf("rules not imported: %+v", svc.Cfg.AutoSwitchRules)
	}
	if svc.Cfg.Language != "zh_cn" || svc.Cfg.SortMode != models.SortNone || svc.Cfg.AtomcodeDir() == "" {
		t.Fatalf("local settings were overwritten: %+v", svc.Cfg)
	}
}
