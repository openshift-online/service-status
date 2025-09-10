package release_inspection

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
)

func MakeEnvironmentReleaseName(environment, release string) string {
	return fmt.Sprintf("%s---%s", environment, release)
}

func SplitEnvironmentReleaseName(name string) (string, string, bool) {
	parts := strings.Split(name, "---")
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func MakeReleaseName(commitTime, sha string) string {
	return fmt.Sprintf("%s-%s", commitTime, sha)
}

func MakeReleaseNameFromCommit(commit object.Commit) string {
	return MakeReleaseName(commit.Committer.When.Format(time.RFC3339), commit.Hash.String()[:5])
}

func SplitReleaseName(name string) (string, time.Time, string, bool) {
	lastDashIndex := strings.LastIndex(name, "-")
	if lastDashIndex == -1 {
		return "", time.Time{}, "", false
	}

	timeString := name[:lastDashIndex]
	sha := name[lastDashIndex+1:]

	parsedTime, err := time.Parse(time.RFC3339, timeString)
	if err != nil {
		return "", time.Time{}, "", false
	}

	return timeString, parsedTime, sha, true
}
