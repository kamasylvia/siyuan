// SiYuan - From thought to insight, with agents
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"strings"
	"testing"

	"github.com/88250/lute/ast"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/siyuan/kernel/conf"
)

func setupPDFEmbedTest(t *testing.T) {
	t.Helper()
	originalConf := Conf
	Conf = NewAppConf()
	Conf.Editor.Store(conf.NewEditor())
	Conf.Export.Store(conf.NewExport())
	t.Cleanup(func() { Conf = originalConf })
}

func parseTestBlock(t *testing.T, markdown string) *ast.Node {
	t.Helper()
	luteEngine := NewLute()
	tree := parse.Parse("", []byte(markdown), luteEngine.ParseOptions)
	node := tree.Root.FirstChild
	if nil == node {
		t.Fatalf("parsed no block from markdown [%s]", markdown)
	}
	return node
}

func TestPDFAssetEmbedSrc(t *testing.T) {
	setupPDFEmbedTest(t)

	node := parseTestBlock(t, "[demo](assets/demo.pdf)")
	src, found := pdfAssetEmbedSrc(node)
	if !found || src != "assets/demo.pdf" {
		t.Fatalf("pdf asset link not detected: src [%s], found %v", src, found)
	}

	node = parseTestBlock(t, "[demo](assets/DEMO.PDF)")
	if _, found = pdfAssetEmbedSrc(node); !found {
		t.Fatal("uppercase pdf suffix not detected")
	}

	node = parseTestBlock(t, "[demo](https://example.com/a.pdf)")
	if _, found = pdfAssetEmbedSrc(node); found {
		t.Fatal("external pdf link should not be embedded")
	}

	node = parseTestBlock(t, "[demo](assets/demo.png)")
	if _, found = pdfAssetEmbedSrc(node); found {
		t.Fatal("non-pdf asset should not be embedded")
	}

	node = parseTestBlock(t, "<iframe src=\"assets/other.pdf\"></iframe>")
	if _, found = pdfAssetEmbedSrc(node); found {
		t.Fatal("existing iframe blocks should be left alone")
	}
}

func TestRenderBlockDOMPDFEmbed(t *testing.T) {
	setupPDFEmbedTest(t)
	luteEngine := NewLute()
	markdown := "before [demo](assets/demo.pdf) after\n\nplain paragraph"
	tree := parse.Parse("", []byte(markdown), luteEngine.ParseOptions)
	var nodes []*ast.Node
	for node := tree.Root.FirstChild; nil != node; node = node.Next {
		nodes = append(nodes, node)
	}

	pdfPreview := Conf.Editor.Load().PDFPreview
	defer Conf.Editor.Store(&conf.Editor{PDFPreview: pdfPreview})

	Conf.Editor.Store(&conf.Editor{PDFPreview: false})
	dom := renderBlockDOMByNodes(nodes, luteEngine)
	if strings.Contains(dom, "NodeIFrame") {
		t.Fatal("pdf embed rendered while the setting is off")
	}

	Conf.Editor.Store(&conf.Editor{PDFPreview: true})
	dom = renderBlockDOMByNodes(nodes, luteEngine)
	if 1 != strings.Count(dom, "NodeIFrame") {
		t.Fatalf("expected exactly one pdf embed, got [%s]", dom)
	}
	if !strings.Contains(dom, "assets/demo.pdf") {
		t.Fatalf("pdf embed src not resolved to asset path, got [%s]", dom)
	}
}
