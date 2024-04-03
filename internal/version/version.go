package version

import (
	"runtime"
	rdebug "runtime/debug"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	GitCommit         string
	GitBranch         string
	GitSummary        string
	BuildDate         string
	AppVersion        string
	BmclibVersion     = bmclibVersion()
	FleetDBAPIVersion = fleetdbAPIVersion()
	GoVersion         = runtime.Version()
)

type Version struct {
	GitCommit         string `json:"git_commit"`
	GitBranch         string `json:"git_branch"`
	GitSummary        string `json:"git_summary"`
	BuildDate         string `json:"build_date"`
	AppVersion        string `json:"app_version"`
	GoVersion         string `json:"go_version"`
	BmclibVersion     string `json:"bmclib_version"`
	FleetDBAPIVersion string `json:"fleetdbapi_version"`
}

func Current() Version {
	return Version{
		GitBranch:         GitBranch,
		GitCommit:         GitCommit,
		GitSummary:        GitSummary,
		BuildDate:         BuildDate,
		AppVersion:        AppVersion,
		GoVersion:         GoVersion,
		BmclibVersion:     BmclibVersion,
		FleetDBAPIVersion: FleetDBAPIVersion,
	}
}

func ExportBuildInfoMetric() {
	buildInfo := promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "flasher_build_info",
			Help: "A metric with a constant '1' value, labeled by branch, commit, summary, builddate, version, Go version from which Flasher was built.",
		},
		[]string{"branch", "commit", "summary", "builddate", "version", "goversion", "fleetDBAPIVersion"},
	)

	buildInfo.WithLabelValues(GitBranch, GitCommit, GitSummary, BuildDate, AppVersion, GoVersion, FleetDBAPIVersion).Set(1)
}

func bmclibVersion() string {
	buildInfo, ok := rdebug.ReadBuildInfo()
	if !ok {
		return ""
	}

	for _, d := range buildInfo.Deps {
		if strings.Contains(d.Path, "bmclib") {
			return d.Version
		}
	}

	return ""
}

func fleetdbAPIVersion() string {
	buildInfo, ok := rdebug.ReadBuildInfo()
	if !ok {
		return ""
	}

	for _, d := range buildInfo.Deps {
		if strings.Contains(d.Path, "fleetdb") {
			return d.Version
		}
	}

	return ""
}
