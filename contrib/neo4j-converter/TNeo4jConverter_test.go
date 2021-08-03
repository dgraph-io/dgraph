package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"text/scanner"
)

func TestParsingHeader(t *testing.T) {
	i := strings.NewReader("my request")
	buf := new(bytes.Buffer)
	require.Error(t, processNeo4jCSV(i, buf), "column '_start' is absent in file")
}

func TestSingleLineFileString(t *testing.T) {
	header := `"_id","_labels","born","name","released","tagline"` +
		`,"title","_start","_end","_type","roles"`
	detail := `"188",":Movie","","","1999","Welcome to the Real World","The Matrix",,,,`
	fileLines := fmt.Sprintf("%s\n%s", header, detail)
	output := `<_:k_188> <_labels> ":Movie" .
<_:k_188> <born> "" .
<_:k_188> <name> "" .
<_:k_188> <released> "1999" .
<_:k_188> <tagline> "Welcome to the Real World" .
<_:k_188> <title> "The Matrix" .
`
	i := strings.NewReader(fileLines)
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	require.Equal(t, output,buf.String() )
}

func TestWholeFile(t *testing.T) {
	goldenFile := "./output.rdf"
	inBuf, _ := ioutil.ReadFile("./example.csv")
	i := strings.NewReader(string(inBuf))
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	//check id
	require.Contains(t, buf.String(), "<_:k_188> <_labels> \":Movie\" .")
	//check facets
	require.Contains(t, buf.String(),
		"<_:k_191> <ACTED_IN> <_:k_188> (  roles=\"\\\"Morpheus\\\"\" )")
	//check link w/o facets
	require.Contains(t, buf.String(), "<_:k_193> <DIRECTED> <_:k_188>")

	//check full file
	expected, err := ioutil.ReadFile(goldenFile)
	if err != nil {
		// Handle error
	}
	isSame := bytes.Equal(expected, buf.Bytes())
	if !isSame {
		fmt.Println("Printing comparison")
		dmp := diffmatchpatch.New()
		diffs := dmp.DiffMain(string(expected), buf.String(), true)
		fmt.Println(dmp.DiffPrettyText(diffs))
	}
	require.True(t, isSame)

}
func TestSingleLineFileWithLineBreakVizzn(t *testing.T) {
	header := `"_id","_labels","actionStrategyId","allSchemas","allowSignup","assignments","audiences","bgColor","calendarId","calendarType","capacity","classificationId","combinationTypeId","contactEmail","contactPreference","contactSms","createdAt","createdBy","currentLocation","currentLocationId","date","days","declaredValue","description","descriptionFieldId","dispatcherGroupId","dispatcherId","displayName","duration","email","emailParts","end","entityClassId","entityId","equipmentIdFieldId","eventType","fieldId","firstName","formId","formioId","formioSubmissionId","fromDay","groupIds","hasMap","id","identityFields","intervalFrequency","isActive","isArchived","isPreferred","isReadOnly","isResourceProvider","jobCode","jobSiteIdFieldId","keyRoleIds","lastName","list_tags","loadTypeIds","loadTypes","locationClassificationId","locationStatus","minuteOfDay","name","nextAlertAt","nextIntervalAt","notes","ownedBy","partnerIds","partners","personIds","priority","requiresFuel","returnToServiceAt","roleIds","scheduleId","scheduleIntervalIds","sharingStatus","siteId","start","startDate","status","styleBg","styleText","subjectIds","tag","teamId","templateId","templates","timezone","trailerCombinationId","trailerIds","trailerOptions","triggerValue","truckId","type","updatedAt","updatedBy","userIds","user_tags","version","year","_start","_end","_type","audienceId","duration","fieldId","inactive","templateId"`
	detail := `"10331",":Site","","","","","","","","","","","","","","","2021-02-04T20:23:26.163Z","T35LTMN675hPzYBa9eizD55ZoLm1","","","","","","Full scope onsite and offsite





","","","","","","","","","uRlHnNBYsgaQyVQluuCjf","","","","","","","","","","","true","Nztiwb-l76brh3401PAeP","","","","false","","","","","","","","[""site""]","","","","","","20-091 Sunnyside","","","","s2aLVSnDvXheCyUzfPK6NCoMtE43","","","","","false","","","","","","","","","active","","","","","Wi4VGIlOPjdin3aL9gC8","","","","","","","","","site","2021-02-04T20:57:22.901Z","wPlqbFJwKBUre431dwlMbnGLov53","","[]","","",,,,,,,,`
	fileLines := fmt.Sprintf("%s\n%s", header, detail)

	i := strings.NewReader(fileLines)
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	fmt.Println(buf.String())
}




func TestSingleLineFileWithLineBreak(t *testing.T) {
	header := `"_id","_labels","born","name","released","tagline"` +
		`,"title","_start","_end","_type","roles"`
	detail := `"188",":Movie","","","1999","Welcome\n to \nthe \"Real\" World","The Matrix",,,,`
	fileLines := fmt.Sprintf("%s\n%s", header, detail)
	output := `<_:k_188> <_labels> ":Movie" .
<_:k_188> <born> "" .
<_:k_188> <name> "" .
<_:k_188> <released> "1999" .
<_:k_188> <tagline> "Welcome\\n to \\nthe \\\"Real\\\" World" .
<_:k_188> <title> "The Matrix" .
`
	i := strings.NewReader(fileLines)
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	out:=buf.String()
	require.Equal(t, output,out )
}

func TestMultiLineFileWithLineBreakWithFacets(t *testing.T) {
	header := `"_id","_labels","born","name","released","tagline"` +
		`,"title","_start","_end","_type","roles"`
	line1 := `"188",":Movie","","","1999","Welcome\n to \n\n\n\n\nthe \"Real\" World","The Matrix",,,,`
	line2 := `"189",":Person","1964","Keanu Reeves","","","",,,,`
	facetLine := `,,,,,,,"189","188","ACTED_IN","[""N\neo""]"`

	fileLines := fmt.Sprintf("%s\n%s\n%s\n%s", header, line1, line2, facetLine)
	output := `<_:k_188> <_labels> ":Movie" .
<_:k_188> <born> "" .
<_:k_188> <name> "" .
<_:k_188> <released> "1999" .
<_:k_188> <tagline> "Welcome\\n to \\nthe \\\"Real\\\" World" .
<_:k_188> <title> "The Matrix" .
`
	i := strings.NewReader(fileLines)
	buf := new(bytes.Buffer)
	processNeo4jCSV(i, buf)
	fmt.Println(buf.String())

	fmt.Println("ignore this",len(output))
	//out:=buf.String()
	//require.Equal(t, output,out )
}

func BenchmarkSampleFile(b *testing.B) {
	inBuf, _ := ioutil.ReadFile("./example.csv")
	i := strings.NewReader(string(inBuf))
	buf := new(bytes.Buffer)
	for k := 0; k < b.N; k++ {
		processNeo4jCSV(i, buf)
		buf.Reset()
	}
}

func TestWholeFileWithQuotedLineBreaks(t *testing.T) {
	//goldenFile := "./output.rdf"
	inBuf, _ := ioutil.ReadFile("./exampleLineBreaks.csv")
	i := strings.NewReader(string(inBuf))
	//buf := new(bytes.Buffer)

	scanner := bufio.NewScanner(i)
	//scanner.Split(TScanLineWithLineBreaks)
	scanner.Scan()
	txt:=scanner.Text()
	fmt.Println(txt)

}

func TestScan(t *testing.T){
	file, _ := os.Open("./exampleLineBreaks.csv")
	r := bufio.NewReader(file)
	var s scanner.Scanner
	s.Init(r)
	count := 0
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		fmt.Printf("%s: %s\n", s.Position, s.TokenText())
		if count>5 {
			break
		}
		count = count + 1
	}
}

func TestReader(t *testing.T){
	file, _ := os.Open("./exampleLineBreaks.csv")
	var r *bufio.Reader
	r = bufio.NewReader(file)
	var lineBuffer bytes.Buffer

	completeLineRead := false
	continueReadingLine := false
	for !completeLineRead {
		continueReadingLine = false
		line,_ := r.ReadString('\n')
		lineBuffer.WriteString(line)
		handler := func(s *scanner.Scanner, msg string) {
			fmt.Println("ERROR")
			fmt.Println(msg)
			if msg == "literal not terminated"{
				continueReadingLine = true
			}
		}

		var s scanner.Scanner
		s.Init(strings.NewReader(line))
		s.Error = handler
		s.Scan()

		if continueReadingLine {
			completeLineRead = false
		} else{
			completeLineRead = true
		}
		//for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		//	fmt.Printf("%s: %s\n", s.Position, s.TokenText())
		//}
	}
	fmt.Println(continueReadingLine)
	fmt.Println(lineBuffer.String())
}

func TestNeo4jFileReader(t *testing.T){
	fileName := "./exampleLineBreaks.csv"
	var nContext Neo4jCSVContext
	nContext.Init(fileName)

	eofReached := false
	nextLine := ""
	count := 0
	for !eofReached {
		nextLine, eofReached = nContext.ProvideNextLine()
		fmt.Println(nextLine)
		count=count  + 1
	}

	require.Equal(t, 2, count)
}

