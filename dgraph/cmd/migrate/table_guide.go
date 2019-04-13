package migrate

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	SEPERATOR = "_"
)

// A KeyGenerator generates the unique blank node label that corresponds to a Dgraph uid.
// Values are passed to the generateKey method in the order of alphabetically sorted columns
type KeyGenerator interface {
	generateKey(info *TableInfo, values []interface{}) string
}

// generate blank node labels using values in the primary key columns
type ColumnKeyGenerator struct {
	primaryKeyIndices []*ColumnIdx
	separator         string
}

type CriteriaFunc func(info *TableInfo, column string) bool

// For example, if the employee table has 3 columns (f_name, l_name, and title),
// where f_name and l_name together form the primary key.
// Then a row with values John (f_name), Doe (l_name), Software Engineer (title)
// would generate a blank node label _:person_John_Doe using values from the columns
// of the primary key in the alphabetic order, i.e. f_name, l_name in this case.
func (g *ColumnKeyGenerator) generateKey(info *TableInfo, values []interface{}) string {
	if g.primaryKeyIndices == nil {
		g.primaryKeyIndices = getColumnIndices(info, func(info *TableInfo, column string) bool {
			return info.columns[column].keyType == PRIMARY
		})
	}

	// use the primary key indices to retrieve values in the current row
	valuesForKey := make([]string, 0)
	for _, columnIndex := range g.primaryKeyIndices {
		valuesForKey = append(valuesForKey, fmt.Sprintf("%v", values[columnIndex.index]))
	}

	return fmt.Sprintf("_:%s%s%s", info.tableName, g.separator,
		strings.Join(valuesForKey, g.separator))
}

// generate blank node labels using a row counter
type CounterKeyGenerator struct {
	rowCounter int
	separator  string
}

func (g *CounterKeyGenerator) generateKey(info *TableInfo, values []interface{}) string {
	g.rowCounter++
	return fmt.Sprintf("_:%s%s%d", info.tableName, g.separator, g.rowCounter)
}

// a ValuesRecorder remembers the mapping between an alias and its blank node label
// For example, if the person table has the (fname, lname) as the primary key,
// and hence the blank node labels are like _:person_<first name value>_<last name value>,
// there are two unique indices on the columns license, and (ssn) respectively.
// For the row fname (John), lname (Doe), license(101), ssn (999-999-9999)
// the Value recorder would remember the following mappings
// _:person_license_101 -> _:person_John_Doe
// _:person_ssn_999-999-9999 -> _:person_John_Doe
// It remembers these mapping so that if another table references the person table through foreign
// keys, it will be able to look up the blank node labels and use it to establish links in Dgraph
type ValuesRecorder interface {
	record(info *TableInfo, values []interface{}, uidLabel string)
	getUidLabel(indexLabel string) string
}

type ForeignKeyValuesRecorder struct {
	referenceToUidLabel       map[string]string
	separator                 string
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
func (r *ForeignKeyValuesRecorder) record(info *TableInfo, values []interface{}, uidLabel string) {
	/*
	if r.referencedByColumnIndices == nil {
		r.referencedByColumnIndices = getColumnIndices(info, func(info *TableInfo, column string) bool {
			return info.columns[column].isForeignKeyTarget
		})
	}

	for _, columnIndex := range r.referencedByColumnIndices {
		referenceLabel := fmt.Sprintf("_:%s%s%s%s%v", info.tableName,
			r.separator, columnIndex.name, r.separator, values[columnIndex.index])
		r.referenceToUidLabel[referenceLabel] = uidLabel
	}
	 */

	for _, constraint := range info.constraintSources {
		// for each foreign key constraint, there should be a mapping
		constraintColumns := getConstraintColumns(constraint)
		constraintColumnIndices := getColumnIndices(info, func(info *TableInfo, column string) bool {
			_, ok := constraintColumns[column]
			return ok
		})

		aliasLabel := getAliasLabel(info, r.separator, constraintColumnIndices, values)
		r.referenceToUidLabel[aliasLabel] = uidLabel
	}
}

func getConstraintColumns(constraint *ForeignKeyConstraint) map[string]interface{} {
	columnNames := make(map[string]interface{})
	for _, part := range constraint.parts {
		columnNames[part.columnName] = struct{}{}
	}
	return columnNames
}

func getAliasLabel(info *TableInfo, separator string, columnIndices []*ColumnIdx,
	values []interface) string {
	columnNameAndIdxes := make([]string, 0)
	for _, columnIdx := range columnIndices {
		columnNameAndIdxes = append(columnNameAndIdxes,
			fmt.Sprintf( "%s%s%v", columnIdx.name, separator, values[columnIdx.index]))
	}

	return fmt.Sprintf("_:%s%s", info.tableName, strings.Join(columnNameAndIdxes, separator))
}

func (r *ForeignKeyValuesRecorder) getUidLabel(indexLabel string) string {
	return r.referenceToUidLabel[indexLabel]
}

type IndexGenerator interface {
	generateDgraphIndices(info *TableInfo) []string
}

// NoneCompositeIndexGenerator generates one Dgraph index per SQL table primary key
// or index, where only the first column in the primary key or index will be used
type NoneCompositeIndexGenerator struct {
	separator string
}

func (g *NoneCompositeIndexGenerator) generateDgraphIndices(info *TableInfo) []string {
	sqlIndexedColumns := getColumnIndices(info, func(info *TableInfo, column string) bool {
		return info.columns[column].keyType != NONE
	})

	dgraphIndexes := make([]string, 0)
	for _, column := range sqlIndexedColumns {
		predicate := fmt.Sprintf("%s%s%s", info.tableName, g.separator, column.name)

		dataType := info.columns[column.name].dataType

		var index string
		if dataType == STRING {
			index = "@index(exact)"
		} else {
			index = fmt.Sprintf("@index(%s)", dataType)
		}

		dgraphIndexes = append(dgraphIndexes, fmt.Sprintf("%s: %s %s .\n",
			predicate, dataType, index))
		/*
		// if this column is a foreign key, we also need to add a new predicate of type uid
		// which will be used to store the link to the remote node
			if _, ok := info.foreignKeyReferences[column.name]; ok {
				dgraphIndexes = append(dgraphIndexes, fmt.Sprintf("%s: %s .\n",
					getLinkPredicate(predicate), UID))
			}
		*/
	}

	for _, constraint := range 
	return dgraphIndexes
}

type PredNameGenerator interface {
	generatePredicateName(info *TableInfo, column string) string
}

type SimplePredNameGenerator struct {
	separator string
}

func (g *SimplePredNameGenerator) generatePredicateName(info *TableInfo, column string) string {
	return fmt.Sprintf("%s%s%s", info.tableName, g.separator, column)
}

type TableGuide struct {
	keyGenerator      KeyGenerator
	valuesRecordor    ValuesRecorder
	indexGenerator    IndexGenerator
	predNameGenerator PredNameGenerator
}

func getKeyGenerator(tableInfo *TableInfo) KeyGenerator {
	// check if the table has primary keys
	primaryKeyIndices := getColumnIndices(tableInfo, func(info *TableInfo, column string) bool {
		return info.columns[column].keyType == PRIMARY
	})

	if len(primaryKeyIndices) > 0 {
		return &ColumnKeyGenerator{
			separator: SEPERATOR,
		}
	}

	return &CounterKeyGenerator{
		separator: SEPERATOR,
	}
}

func getTableGuides(tables map[string]*TableInfo) map[string]*TableGuide {
	tableGuides := make(map[string]*TableGuide)
	for table, tableInfo := range tables {
		guide := &TableGuide{
			keyGenerator: getKeyGenerator(tableInfo),
			valuesRecordor: &ForeignKeyValuesRecorder{
				referenceToUidLabel: make(map[string]string),
				separator:           SEPERATOR,
			},
			indexGenerator: &NoneCompositeIndexGenerator{
				separator: SEPERATOR,
			},
			predNameGenerator: &SimplePredNameGenerator{
				separator: SEPERATOR,
			},
		}

		tableGuides[table] = guide
	}
	return tableGuides
}
