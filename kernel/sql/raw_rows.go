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

package sql

import "database/sql"

// SelectRawRows 执行只读 SELECT，返回有序列名与字符串化行数据。
// 供嵌入块 raw 渲染模式输出任意查询结果（聚合统计等非块行查询）。
func SelectRawRows(boxID, stmt string, limit int) (columns []string, rows [][]string, err error) {
	resultRows, err := queryForBox(boxID, stmt)
	if err != nil {
		return
	}
	defer resultRows.Close()
	columns, err = resultRows.Columns()
	if err != nil {
		return
	}
	values := make([]any, len(columns))
	for i := range values {
		values[i] = new(sql.RawBytes)
	}
	for resultRows.Next() {
		if scanErr := resultRows.Scan(values...); nil != scanErr {
			err = scanErr
			return
		}
		row := make([]string, len(columns))
		for i, value := range values {
			rawBytes := value.(*sql.RawBytes)
			if rawBytes == nil {
				row[i] = ""
			} else {
				row[i] = string(*rawBytes)
			}
		}
		rows = append(rows, row)
		if 0 < limit && len(rows) >= limit {
			break
		}
	}
	return
}
