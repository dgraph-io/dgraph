package migrate

import (
	"fmt"
	"sort"
	"strings"
)

// a KeyGenerator generates the unique label that corresponds to a Dgraph uid
// values are passed to the generateKey method in the order of alphabetically sorted columns
// For example, if the person table has the fname and last name combined as the primary key
// then a row John, Doe would passed to the key generator would return _:person_fname_John_lname:Doe
type KeyGenerator interface {
	generateKey(info *TableInfo, values []interface{}) string
}

type ColumnIdx struct {
	name  string // the column name
	index int    // the column index
}

// generate uidLabels using values in the primary key columns
type ColumnKeyGenerator struct {
	primaryKeyIndices []*ColumnIdx
	separator         string
}

type CriteriaFunc func(info *TableInfo, column string) bool

// getColumnIndices first sort the columns in the table alphabetically, and then
// returns the indices of primary key columns
func getColumnIndices(info *TableInfo,
	criteria CriteriaFunc) []*ColumnIdx {
	columns := make([]string, 0)
	for _, columnInfo := range info.columns {
		columns = append(columns, columnInfo.name)
	}

	// sort the column names alphabetically
	sort.Slice(columns, func(i, j int) bool {
		return columns[i] < columns[j]
	})

	indices := make([]*ColumnIdx, 0)
	for i, column := range columns {
		if criteria(info, column) {
			indices = append(indices, &ColumnIdx{
				name:  column,
				index: i,
			})
		}
	}
	return indices
}

func (g ColumnKeyGenerator) generateKey(info *TableInfo, values []interface{}) string {
	if g.primaryKeyIndices == nil {
		g.primaryKeyIndices = getColumnIndices(info, func(info *TableInfo, column string) bool {
			return info.columns[column].keyType == PRIMARY
		})
	}

	// use the primary key indices to retrieve values in the current row
	valuesForKey := make([]string, 0)
	for _, columnIndex := range g.primaryKeyIndices {
		valuesForKey = append(valuesForKey, fmt.Sprintf("%s", values[columnIndex.index]))
	}

	return fmt.Sprintf("_:%s%s%s", info.tableName, g.separator,
		strings.Join(valuesForKey, g.separator))
}

// generate uidLabels using a row counter
type CounterKeyGenerator struct {
	rowCounter int
	separator  string
}

func (g CounterKeyGenerator) generateKey(info *TableInfo, values []interface{}) string {
	g.rowCounter++
	return fmt.Sprintf("_:%s%s%d", info.tableName, g.separator, g.rowCounter)
}

// a ValuesRecorder remembers the mapping between an alias and its uid label
// For example, if the person table has the id as the primary key (hence uidLabel _:person_id_xx)
// and there are two unique indices on the columns (fname, lname), and (ssn) respectively.
// then for the row id (101), fname (John), lname (Doe), ssn (999-999-9999)
// the Value recorder would remember the following mapping
// _:person_fname_John_lname_Doe -> _:person_101
// _:person_ssn_999-999-9999 -> _:person:101
// we remember the mapping so that if another table references the person table through foreign keys
// we will be able to look up the uidLabel and use it to establish links in Dgraph
type ValuesRecorder interface {
	record(info *TableInfo, values []interface{}, uidLabel string)
	getUidLabel(indexLabel string) string
}

type ForeignKeyValuesRecorder struct {
	indexToUid map[string]string
}

/*
TODO: for now we do NOT support composite foreign keys
For example, if we have the following two tables where <fname, lname> combined serves as the
foreign key,

create table person (
fname varchar(50),
lname varchar(50),
INDEX (fname, lname)
);

create table role (
title varchar(50),
p_fname varchar(50),
p_lname varchar(50),
FOREIGN KEY (p_fname, p_lname) REFERENCES person (fname, lname)
);

the tool will treat them as two different foreign keys, where the p_fname references person fname,
and p_lname references person lname.
*/
func (r ForeignKeyValuesRecorder) record(info *TableInfo, values []interface{}, uidLabel string) {

}

func (r ForeignKeyValuesRecorder) getUidLabel(indexLabel string) string {
	return ""
}

type TableGuide struct {
	keyGenerator   KeyGenerator
	valuesRecorder ValuesRecorder
}
