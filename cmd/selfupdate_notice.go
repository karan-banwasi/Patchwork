package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/httputil"
	"github.com/spf13/cobra"
)

const (
	selfUpdateCacheTTL      = 24 * time.Hour
	passiveCheckHTTPTimeout = 2 * time.Second
	passiveCheckGraceWait   = 1500 * time.Millisecond
)

var selfUpdateNoticeClient *http.Client

// SelfUpdateCache stores the cached result of the last passive self-update check.
type SelfUpdateCache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
	Channel       string    `json:"channel,omitempty"`
	ReleaseURL    string    `json:"release_url,omitempty"`
}

// SelfUpdateCachePath returns the path to the passive self-update cache file.
func SelfUpdateCachePath() string {
	if custom := os.Getenv("PATCHWORK_SELFUPDATE_CACHE"); custom != "" {
		return custom
	}
	return filepath.Join(filepath.Dir(config.CachePath()), "selfupdate-cache.json")
}

func isEnvTruthy(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	return v == "1" || strings.EqualFold(v, "true")
}

func loadSelfUpdateCache(channel string) (*SelfUpdateCache, bool) {
	data, err := os.ReadFile(SelfUpdateCachePath())
	if err != nil {
		return nil, false
	}
	var cache SelfUpdateCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, false
	}
	if cache.LatestVersion == "" || cache.CheckedAt.IsZero() {
		return nil, false
	}
	if time.Since(cache.CheckedAt) > selfUpdateCacheTTL {
		return nil, false
	}
	if cache.Channel == "" {
		if channel != channelStable {
			return nil, false
		}
	} else if !strings.EqualFold(cache.Channel, channel) {
		return nil, false
	}
	return &cache, true
}

func saveSelfUpdateCache(cache *SelfUpdateCache) error {
	if cache == nil {
		return nil
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	path := SelfUpdateCachePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "selfupdate-cache-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true

	_ = os.Chmod(tmpName, 0600)
	return os.Rename(tmpName, path)
}

func isBoolFlagSet(cmd *cobra.Command, name string) bool {
	if cmd == nil {
		return false
	}
	v, err := cmd.Flags().GetBool(name)
	return err == nil && v
}

func shouldSuppressUpdateNotice(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	if strings.TrimSpace(version) == "" || version == "dev" {
		return true
	}
	if isEnvTruthy("PATCHWORK_OFFLINE") || isEnvTruthy("PATCHWORK_NO_UPDATE_CHECK") {
		return true
	}
	if isBoolFlagSet(cmd, "json") || isBoolFlagSet(cmd, "quiet") || isBoolFlagSet(cmd, "offline") {
		return true
	}
	for cur := cmd; cur != nil && cur.Parent() != nil; cur = cur.Parent() {
		switch cur.Name() {
		case "self-update", "update", "completion", "version", "help":
			return true
		}
		if cur.CalledAs() == "update" {
			return true
		}
	}
	return false
}

type selfUpdateNoticeHandle struct {
	cancel context.CancelFunc
	done   chan *SelfUpdateCache
	cached *SelfUpdateCache
}

func startPassiveSelfUpdateCheck(cmd *cobra.Command) *selfUpdateNoticeHandle {
	if shouldSuppressUpdateNotice(cmd) {
		return nil
	}

	channel := channelStable
	if cfg, err := config.Load(); err == nil && cfg != nil && cfg.Defaults.Channel != "" {
		if ch, err := normalizeChannel(cfg.Defaults.Channel); err == nil && ch != "" {
			channel = ch
		}
	}

	if cached, ok := loadSelfUpdateCache(channel); ok {
		return &selfUpdateNoticeHandle{cached: cached}
	}

	baseCtx := cmd.Context()
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, passiveCheckHTTPTimeout)
	done := make(chan *SelfUpdateCache, 1)

	client := selfUpdateNoticeClient
	if client == nil {
		client = httputil.NewClient(
			httputil.WithTimeout(passiveCheckHTTPTimeout),
			httputil.WithMaxRetries(0),
		)
	}
	apiBaseURL := selfUpdateAPIBaseURL
	token := githubTokenFromEnv()

	go func() {
		defer cancel()
		defer close(done)

		rel, err := fetchTargetRelease(ctx, client, apiBaseURL, channel, token)
		if err != nil || rel == nil || ctx.Err() != nil {
			return
		}
		entry := &SelfUpdateCache{
			CheckedAt:     time.Now().UTC(),
			LatestVersion: rel.TagName,
			Channel:       channel,
			ReleaseURL:    rel.HTMLURL,
		}
		if ctx.Err() == nil {
			_ = saveSelfUpdateCache(entry)
			done <- entry
		}
	}()

	return &selfUpdateNoticeHandle{
		cancel: cancel,
		done:   done,
	}
}

func finishPassiveSelfUpdateCheck(cmd *cobra.Command, handle *selfUpdateNoticeHandle) {
	if handle == nil {
		return
	}
	if shouldSuppressUpdateNotice(cmd) {
		if handle.cancel != nil {
			handle.cancel()
		}
		return
	}

	entry := handle.cached
	if entry == nil && handle.done != nil {
		timer := time.NewTimer(passiveCheckGraceWait)
		select {
		case res := <-handle.done:
			entry = res
		case <-timer.C:
			if handle.cancel != nil {
				handle.cancel()
			}
		}
		timer.Stop()
	}
	if handle.cancel != nil {
		handle.cancel()
	}

	if entry != nil && entry.LatestVersion != "" && isNewerVersion(version, entry.LatestVersion) {
		fmt.Fprintf(cmd.ErrOrStderr(), "\nA new version of Patchwork is available: %s -> %s (run 'pw self-update' to upgrade)\n", version, entry.LatestVersion)
	}
}
