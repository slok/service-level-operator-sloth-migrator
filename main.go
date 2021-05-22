package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"regexp"

	"github.com/ghodss/yaml"
	slothv1 "github.com/slok/sloth/pkg/kubernetes/api/sloth/v1"
	monitoringv1alpha1 "github.com/spotahome/service-level-operator/pkg/apis/monitoring/v1alpha1"
	"gopkg.in/alecthomas/kingpin.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type config struct {
	SpecFile      string
	IgnoreDisable bool
	OutDir        string
}

func newConfig(args []string) (*config, error) {
	var c config
	app := kingpin.New("service-level-operator-sloth-migrator", "Migrate SLOs from service-level-operator to sloth.")
	app.DefaultEnvars()
	app.Flag("slos", "Service-level-operator SLOs CR YAML file (accepts ServiceLevel and ServiceLevelList)").Required().StringVar(&c.SpecFile)
	app.Flag("out", "Directory to place the generated Sloth CRs").Required().StringVar(&c.OutDir)
	app.Flag("ignore-disable", "If SLO have the disable field to true, it will not migrate those.").BoolVar(&c.IgnoreDisable)

	// Parse commandline.
	_, err := app.Parse(args[1:])
	if err != nil {
		return nil, fmt.Errorf("invalid command configuration: %w", err)
	}

	return &c, nil
}

func run(ctx context.Context) error {
	config, err := newConfig(os.Args)
	if err != nil {
		return fmt.Errorf("could not load configuration: %w", err)
	}

	// Load service level operator SLOs from file.
	data, err := os.ReadFile(config.SpecFile)
	if err != nil {
		return fmt.Errorf("could not read service-level SLOs: %w", err)
	}

	opSLs, err := loadSLOperatorSLs(data)
	if err != nil {
		return fmt.Errorf("could not load SLOs: %w", err)
	}

	// Map SLOs.
	slothSLs := make([]slothv1.PrometheusServiceLevel, 0, len(opSLs))
	for _, opSL := range opSLs {
		slothSL, err := mapSLOperatorToSloth(*config, opSL)
		if err != nil {
			return fmt.Errorf("could not map service-level-operator CR to Sloth CR: %w", err)
		}

		if slothSL == nil {
			fmt.Printf("Ignoring %s service level: 0 service levels\n", opSL.Name)
			continue
		}

		slothSLs = append(slothSLs, *slothSL)
	}

	if len(opSLs) == 0 {
		return fmt.Errorf("0 SLOs loaded")
	}

	// Generate Sloth SLOs.
	for _, slothSL := range slothSLs {
		err := storeSlothSLs(*config, slothSL)
		if err != nil {
			return fmt.Errorf("could not store Sloth SL: %w", err)
		}
	}

	return nil
}

func loadSLOperatorSLs(data []byte) ([]monitoringv1alpha1.ServiceLevel, error) {
	// Try loading a List of SLOs.
	slList := &monitoringv1alpha1.ServiceLevelList{}
	err := yaml.Unmarshal(data, &slList)
	if err != nil {
		return nil, err
	}

	if len(slList.Items) != 0 {
		return slList.Items, nil
	}

	// Try loading a single one.
	sl := &monitoringv1alpha1.ServiceLevel{}
	err = yaml.Unmarshal(data, &sl)
	if err != nil {
		return nil, err
	}

	return []monitoringv1alpha1.ServiceLevel{*sl}, nil

}

func mapSLOperatorToSloth(config config, sl monitoringv1alpha1.ServiceLevel) (*slothv1.PrometheusServiceLevel, error) {
	if len(sl.Spec.ServiceLevelObjectives) == 0 {
		return nil, nil
	}

	slos := []slothv1.SLO{}

	for _, slo := range sl.Spec.ServiceLevelObjectives {
		if config.IgnoreDisable && slo.Disable {
			continue
		}

		slos = append(slos, slothv1.SLO{
			Name:        slo.Name,
			Objective:   slo.AvailabilityObjectivePercent,
			Description: slo.Description,
			Labels:      slo.Output.Prometheus.Labels,
			SLI: slothv1.SLI{
				Events: &slothv1.SLIEvents{
					ErrorQuery: replaceWindow(slo.ServiceLevelIndicator.Prometheus.ErrorQuery),
					TotalQuery: replaceWindow(slo.ServiceLevelIndicator.Prometheus.TotalQuery),
				},
			},
			Alerting: slothv1.Alerting{
				PageAlert:   slothv1.Alert{Disable: true},
				TicketAlert: slothv1.Alert{Disable: true},
			},
		})
	}

	return &slothv1.PrometheusServiceLevel{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "sloth.slok.dev/v1",
			Kind:       "PrometheusServiceLevel",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        sl.Name,
			Namespace:   sl.Namespace,
			Labels:      sl.Labels,
			Annotations: sl.Annotations,
		},
		Spec: slothv1.PrometheusServiceLevelSpec{
			Service: sl.Name,
			SLOs:    slos,
		},
	}, nil
}

func storeSlothSLs(config config, slothSL slothv1.PrometheusServiceLevel) error {
	data, err := yaml.Marshal(slothSL)
	if err != nil {
		return fmt.Errorf("could not marshal Sloth service level: %w", err)
	}

	fName := fmt.Sprintf("_gen_%s_%s.yaml", slothSL.Namespace, slothSL.Name)
	fName = path.Join(config.OutDir, fName)
	err = os.WriteFile(fName, data, 0644)
	if err != nil {
		return fmt.Errorf("could not write Sloth service level: %w", err)
	}

	fmt.Printf("[*] %s: %s\n", slothSL.Name, fName)

	return nil
}

var rangeRegex = regexp.MustCompile(`\[\d[smhd]\]`)

func replaceWindow(s string) string {
	return rangeRegex.ReplaceAllString(s, "[{{ .window }}]")
}

func main() {
	ctx := context.Background()
	err := run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err)
		os.Exit(42)
	}
}
