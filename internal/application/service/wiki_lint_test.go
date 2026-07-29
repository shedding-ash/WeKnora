package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type wikiLintKBServiceStub struct {
	interfaces.KnowledgeBaseService
	kb *types.KnowledgeBase
}

func (s *wikiLintKBServiceStub) GetKnowledgeBaseByIDOnly(context.Context, string) (*types.KnowledgeBase, error) {
	return s.kb, nil
}

func newWikiLintTestService(t *testing.T) (context.Context, *WikiLintService, interfaces.WikiPageService) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WikiFolder{}, &types.WikiPage{}, &types.WikiPageRevision{}))

	ctx := context.Background()
	repo := repository.NewWikiPageRepository(db)
	wikiSvc := NewWikiPageService(repo, nil, nil, nil, nil)
	kbSvc := &wikiLintKBServiceStub{kb: &types.KnowledgeBase{
		ID:               "kb-lint",
		IndexingStrategy: types.IndexingStrategy{WikiEnabled: true},
	}}
	return ctx, NewWikiLintService(wikiSvc, kbSvc, nil), wikiSvc
}

func TestRunLintDetectsStaleInLinks(t *testing.T) {
	ctx, lintSvc, wikiSvc := newWikiLintTestService(t)
	now := time.Now()
	_, err := wikiSvc.CreatePage(ctx, &types.WikiPage{
		ID: "page-linked", TenantID: 1, KnowledgeBaseID: "kb-lint", Slug: "entity/live",
		Title: "Live", PageType: types.WikiPageTypeEntity, Status: types.WikiPageStatusPublished,
		Content:   "This page has enough content to avoid the empty-content warning in lint.",
		InLinks:   types.StringArray{"summary/deleted"},
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)

	report, err := lintSvc.RunLint(ctx, "kb-lint")
	require.NoError(t, err)

	var found bool
	for _, issue := range report.Issues {
		if issue.Type == LintIssueBrokenLink && issue.PageSlug == "entity/live" && issue.TargetSlug == "summary/deleted" {
			found = true
			require.True(t, issue.AutoFixable)
		}
	}
	require.True(t, found, "expected stale inbound link to be reported")
}

func TestAutoFixRebuildsStaleInLinks(t *testing.T) {
	ctx, lintSvc, wikiSvc := newWikiLintTestService(t)
	_, err := wikiSvc.CreatePage(ctx, &types.WikiPage{
		ID: "page-linked", TenantID: 1, KnowledgeBaseID: "kb-lint", Slug: "entity/live",
		Title: "Live", PageType: types.WikiPageTypeEntity, Status: types.WikiPageStatusPublished,
		Content: "This page has enough content to avoid the empty-content warning in lint.",
		InLinks: types.StringArray{"summary/deleted"},
	})
	require.NoError(t, err)

	fixed, err := lintSvc.AutoFix(ctx, "kb-lint")
	require.NoError(t, err)
	require.Equal(t, 1, fixed)

	page, err := wikiSvc.GetPageBySlug(ctx, "kb-lint", "entity/live")
	require.NoError(t, err)
	require.Empty(t, page.InLinks)
}
