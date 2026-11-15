// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"strings"
	"testing"

	"github.com/siyuan-note/siyuan/kernel/conf"
	"github.com/siyuan-note/siyuan/kernel/filesys"
	"github.com/siyuan-note/siyuan/kernel/treenode"
)

// Reproduces the empty MCP export reported on the NAS instance: the MCP tool
// kernel/mcp/tools/export.go calls model.ExportMarkdownContent with fixed
// options and got "" for every document, while the kernel zip export
// (/api/export/exportMd) worked for the same documents.
func TestExportMarkdownContentMcpOptionsNonEmpty(t *testing.T) {
	fixture := setupFileOperationTest(t)
	Conf.Export.Store(conf.NewExport())
	Conf.Editor.Store(conf.NewEditor())
	Conf.Search.Store(conf.NewSearch())

	docPath := "/20260718000009-abcexp1.sy"
	tree := treenode.NewTree(fixture.box.ID, docPath, "/McpExportRepro", "McpExportRepro")
	rootID := tree.Root.ID
	if _, err := filesys.WriteTree(tree); err != nil {
		t.Fatalf("write test tree failed: %v", err)
	}
	treenode.UpsertBlockTree(tree)
	t.Cleanup(func() {
		treenode.RemoveBlockTreesByPathPrefix(fixture.box.ID, docPath)
	})

	// Exactly the options kernel/mcp/tools/export.go exportMd passes.
	hPath, content := ExportMarkdownContent(rootID, &ExportOptions{
		BlockRefMode:   new(4),
		BlockEmbedMode: new(0),
		MarkdownYFM:    new(true),
		AddTitle:       new(false),
		Render: ExportRenderOptions{
			FillCSSVar:         false,
			AdjustHeadingLevel: false,
			ImgTag:             false,
		},
	})

	if content == "" {
		t.Fatalf("ExportMarkdownContent returned empty content (hPath=%q) — MCP export regression reproduced", hPath)
	}
	if !strings.Contains(hPath, "McpExportRepro") {
		t.Fatalf("unexpected hPath: %q", hPath)
	}
	t.Logf("hPath=%q content=%.120s", hPath, content)
}
