package main

import (
	"fmt"
	"issuefinder"
	"log"
)

var fails int

func integrityFail(s string) {
	fails++
	log.Printf("Integrity check failed!  %s", s)
}

func validateLen(thing string, a, b int) {
	if a == b {
		return
	}
	integrityFail(fmt.Sprintf("The %s lengths don't match; real finder had %d; cache data had %d", thing, a, b))
}

func testIntegrity(finderA *issuefinder.Finder, cacheFile string) {
	fails = 0
	log.Printf("Reading cached file to verify integrity")
	var finderB, err = issuefinder.Deserialize(cacheFile)
	if err != nil {
		integrityFail(fmt.Sprintf("Unable to deserialize the cached file: %s", err))
	}

	log.Printf("Testing deserialized finder against live finder")
	validateLen("issue", len(finderA.Issues), len(finderB.Issues))
	validateLen("title", len(finderA.Titles), len(finderB.Titles))
	validateLen("batch", len(finderA.Batches), len(finderB.Batches))
	validateLen("error", len(finderA.Errors.Errors), len(finderB.Errors.Errors))

	finderA.Issues.SortByKey()
	finderB.Issues.SortByKey()
	for i, issueA := range finderA.Issues {
		var issueB = finderB.Issues[i]
		var tsvA, tsvB = issueA.TSV(), issueB.TSV()
		if tsvA != tsvB {
			integrityFail(fmt.Sprintf("Issues[%d] don't match: real: %#v cache: %#v", i, tsvA, tsvB))
			if fails > 5 {
				break
			}
		}
	}

	if fails == 0 {
		log.Printf("Cache verified")
	}
}
