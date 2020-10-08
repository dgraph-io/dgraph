package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var TOKEN = os.Getenv("TEAMCITY_TOKEN")

var opts options

const TEAMCITY_BASEURL = "https://teamcity.dgraph.io"

type options struct {
	Days int
}

type AllBuilds struct {
	Count    int    `json:"count"`
	Href     string `json:"href"`
	NextHref string `json:"nextHref"`
	Builds   []struct {
		ID            int    `json:"id"`
		BuildTypeId   string `json:buildTypeId`
		Number        string `json:number`
		Status        string `json:status`
		State         string `json:state`
		Composite     bool   `json:composite`
		BranchName    string `json:branchName`
		DefaultBranch bool   `json:defaultBranch`
		Href          string `json:href`
		WebUrl        string `json:webUrl`
	} `json:"build"`
}

type BuildData struct {
	Number string
	ID     int
	Href   string
}

type AllTestsResponse struct {
	Count          int    `json:"count"`
	Href           string `json:"href"`
	NextHref       string `json:"nextHref"`
	TestOccurrence []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Href   string `json:"href"`
	} `json:"testOccurrence"`
}

type TestStats struct {
	Name      string
	TotalRuns int
	Success   int
	Failure   int
}

type TestData struct {
	Status TestStatus
}

type FlakyStats struct {
	Percent float64
	Name    string
}

type TestStatus string

const (
	SUCCESS TestStatus = "SUCCESS"
	FAILURE            = "FAILURE"
	IGNORED            = "IGNORED"
)

func doGetRequest(url string) []byte {
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	request.Header.Add("Authorization", TOKEN)
	request.Header.Add("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
		panic(err)
	}
	if err != nil {
		panic(err)
	}
	bodyBytes, err := ioutil.ReadAll(resp.Body)

	return bodyBytes
}

// Fetch the status of all the tests that ran for the given buildId
func fetchTestsForBuild(buildID int, ch chan<- map[string]TestData) {
	url := fmt.Sprintf(TEAMCITY_BASEURL+"/app/rest/testOccurrences?locator=build:id:%d", buildID)
	testDataMap := make(map[string]TestData)
	for {
		bodyBytes := doGetRequest(url)
		var alltests AllTestsResponse
		err := json.Unmarshal(bodyBytes, &alltests)

		if err != nil {
			panic(err)
		}
		for i := 0; i < len(alltests.TestOccurrence); i++ {
			var testData TestData
			if alltests.TestOccurrence[i].Status == "SUCCESS" {
				testData.Status = SUCCESS
			} else if alltests.TestOccurrence[i].Status == "FAILURE" {
				testData.Status = FAILURE
			} else {
				testData.Status = IGNORED
			}
			testDataMap[alltests.TestOccurrence[i].Name] = testData
		}
		if len(alltests.NextHref) == 0 {
			break
		} else {
			url = fmt.Sprintf("%s%s", TEAMCITY_BASEURL, alltests.NextHref)
		}
	}
	ch <- testDataMap
}

func fetchAllBuildsSince(buildType string, date string) []BuildData {
	url := fmt.Sprintf("%s/app/rest/builds/?locator=branch:refs/heads/master,buildType:%s,sinceDate:%s", TEAMCITY_BASEURL, buildType, date)
	url = strings.ReplaceAll(url, "+", "%2B")
	var buildDatas []BuildData
	for {
		bodyBytes := doGetRequest(url)
		var allBuilds AllBuilds
		err := json.Unmarshal(bodyBytes, &allBuilds)

		if err != nil {
			panic(err)
		}

		for i := 0; i < len(allBuilds.Builds); i++ {
			var buildData BuildData
			buildData.Href = allBuilds.Builds[i].Href
			buildData.Number = allBuilds.Builds[i].Number
			buildData.ID = allBuilds.Builds[i].ID
			buildDatas = append(buildDatas, buildData)
		}
		if len(allBuilds.NextHref) == 0 {
			break
		} else {
			url = fmt.Sprintf("%s%s", TEAMCITY_BASEURL, allBuilds.NextHref)
		}
	}

	return buildDatas
}

func outputTestsStats(buildType string, days int) {
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	sinceString := since.Format("20060102T150405+0000")

	buildDataList := fetchAllBuildsSince(buildType, sinceString)

	// Get the tests that ran on the last build
	if len(buildDataList) == 0 {
		log.Fatalln("No builds found")
	}
	ch := make(chan map[string]TestData)

	// Get the tests for the latest build first
	go fetchTestsForBuild(buildDataList[0].ID, ch)
	latestTestsMap := <-ch
	testStatsMap := make(map[string]TestStats)

	// For the tests that ran in the latest run, update the stats
	for testName := range latestTestsMap {
		var testStats TestStats
		testStats.Name = testName
		if latestTestsMap[testName].Status == SUCCESS {
			testStats.Success++
		} else if latestTestsMap[testName].Status == FAILURE {
			testStats.Failure++
		}
		testStats.TotalRuns++
		testStatsMap[testName] = testStats
	}

	// Compute test stats for all the builds before the latest build
	for i := 1; i < len(buildDataList); i++ {
		go fetchTestsForBuild(buildDataList[i].ID, ch)
	}

	for i := 1; i < len(buildDataList); i++ {
		currentTestsMap := <-ch
		for k := range latestTestsMap {
			test, found := currentTestsMap[k]
			if !found {
				continue
			}
			var testStats = testStatsMap[k]
			if test.Status == SUCCESS {
				testStats.Success++
			} else if test.Status == FAILURE {
				testStats.Failure++
			}
			testStats.TotalRuns++
			testStatsMap[k] = testStats
		}
	}
	// Sort the test in ascending order of flakiness = failures / total runs
	var allFlakyTests []FlakyStats
	for k := range latestTestsMap {
		var flakyStats FlakyStats
		flakyStats.Name = k
		flakyStats.Percent = float64(testStatsMap[k].Failure) / float64(testStatsMap[k].TotalRuns)
		allFlakyTests = append(allFlakyTests, flakyStats)
	}
	sort.Slice(allFlakyTests, func(i, j int) bool {
		return allFlakyTests[i].Percent > allFlakyTests[j].Percent
	})
	println("Tests that have failed:")
	for i := 0; i < len(allFlakyTests); i++ {
		testStat := testStatsMap[allFlakyTests[i].Name]
		if testStat.Failure == 0 {
			break
		}
		fmt.Printf("%s Failures=%d  Total Runs=%d\n", allFlakyTests[i].Name, testStat.Failure, testStat.TotalRuns)
	}
}

func main() {
	var days int
	var buildType string
	var cmd = &cobra.Command{
		Use:     "test_stats",
		Short:   "Tests stats from TeamCity",
		Long:    "Aggregate stats for tests that run on TeamCity",
		Example: "$ teamcity test_stats -d=30 -b=Dgraph_Ci # fetches stats for last month",
		Run: func(cmd *cobra.Command, args []string) {
			outputTestsStats(buildType, days)
		},
	}
	cmd.Flags().IntVarP(&days, "days", "d", 7, "Past days for which stats are to be computed")
	cmd.Flags().StringVarP(&buildType, "build_type", "b", "Dgraph_Ci", "Build Type for which stats need to be computed")

	var rootCmd = &cobra.Command{Use: "teamcity"}
	rootCmd.AddCommand(cmd)
	rootCmd.Execute()
}
