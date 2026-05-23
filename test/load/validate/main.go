package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	yamlreport "github.com/maansaake/arbiter/pkg/report/yaml"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		errorLogPath string
		reportPath   string
	)

	flag.StringVar(&errorLogPath, "error-log", "", "Path to Arbiter error.log.")
	flag.StringVar(&reportPath, "report", "", "Path to Arbiter report.yaml.")
	flag.Parse()

	if errorLogPath == "" {
		exit(errors.New("error-log path is required"))
	}
	if reportPath == "" {
		exit(errors.New("report path is required"))
	}

	if err := validateErrorLog(errorLogPath); err != nil {
		exit(err)
	}

	if err := validateReport(reportPath); err != nil {
		exit(err)
	}
}

func validateErrorLog(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat error log: %w", err)
	}
	if info.Size() > 0 {
		return fmt.Errorf("arbiter error log is not empty: %s", path)
	}
	return nil
}

func validateReport(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read report: %w", err)
	}

	var rep yamlreport.Report
	if err := yaml.Unmarshal(contents, &rep); err != nil {
		return fmt.Errorf("decode report: %w", err)
	}

	var failures []string
	for moduleName, moduleReport := range rep.Modules {
		for operationName, op := range moduleReport.Operations {
			if op.NOK > 0 {
				failures = append(
					failures,
					fmt.Sprintf("%s/%s has %d nok out of %d executions", moduleName, operationName, op.NOK, op.Executions),
				)
			}
		}
	}

	if len(failures) > 0 {
		return fmt.Errorf("arbiter report contains failed operations: %s", strings.Join(failures, "; "))
	}

	return nil
}

func exit(err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
