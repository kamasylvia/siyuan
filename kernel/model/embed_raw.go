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
	"path"
	"strings"

	"github.com/siyuan-note/logging"
	"github.com/siyuan-note/siyuan/kernel/sql"
	"github.com/siyuan-note/siyuan/kernel/treenode"
)

type EmbedRawTable struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// SearchEmbedBlockRawInBox 执行嵌入块的原始只读 SQL（语句已在上游完成只读校验），返回原始行表格。
// 用于 raw 渲染模式输出聚合统计等非块行查询结果。
func SearchEmbedBlockRawInBox(embedBlockID, stmt, boxID string, limit int) (ret []*EmbedBlock) {
	ret = []*EmbedBlock{}
	columns, rows, err := sql.SelectRawRows(boxID, stmt, limit)
	if err != nil {
		logging.LogWarnf("embed raw query failed: %s", err)
		return
	}
	ret = append(ret, &EmbedBlock{
		Block:   newEmbedRawBlock(embedBlockID),
		RawRows: &EmbedRawTable{Columns: columns, Rows: rows},
	})
	return
}

// RecentDocsEmbedBlock 返回最近浏览的文档块列表：按浏览时间倒序，范围限定为嵌入块所属笔记本。
// 最近浏览数据存于 storage 而非 SQLite，无法经 SQL 查询，故由内核组装；块 ID 供前端行点击跳转。
func RecentDocsEmbedBlock(embedBlockID string, limit int) (ret []*EmbedBlock) {
	ret = []*EmbedBlock{}
	if 1 > limit {
		limit = 10
	}
	embedBt := treenode.GetBlockTree(embedBlockID)
	if nil == embedBt {
		return
	}
	recentDocs, err := GetRecentDocs("viewedAt")
	if err != nil {
		return
	}
	boxNames := Conf.BoxNames([]string{embedBt.BoxID})
	boxName := boxNames[embedBt.BoxID]
	rootID := embedBt.RootID
	subtreeMark := "/" + rootID + "/"
	for _, doc := range recentDocs {
		docBt := treenode.GetBlockTree(doc.RootID)
		if nil == docBt || docBt.BoxID != embedBt.BoxID || doc.RootID == rootID {
			continue
		}
		// 仅展示该父文档子树下的文档（路径包含父文档 ID 目录段）
		if !strings.Contains(docBt.Path, subtreeMark) {
			continue
		}
		title := doc.Title
		if "" == title {
			title = path.Base(docBt.HPath)
		}
		ret = append(ret, &EmbedBlock{
			Block: &Block{
				ID:      doc.RootID,
				Box:     embedBt.BoxID,
				HPath:   boxName + docBt.HPath,
				Content: title,
				Type:    "query_embed",
			},
		})
		if len(ret) >= limit {
			break
		}
	}
	return
}

func newEmbedRawBlock(embedBlockID string) *Block {
	return &Block{ID: embedBlockID, Type: "query_embed", Content: ""}
}
