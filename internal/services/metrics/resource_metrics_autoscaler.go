package metrics

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/philips-software/go-hsdp-api/console"
	"github.com/philips-software/terraform-provider-hsdp/internal/config"
)

func ResourceMetricsAutoscaler() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceMetricsAutoscalerCreate,
		ReadContext:   resourceMetricsAutoscalerRead,
		UpdateContext: resourceMetricsAutoscalerUpdate,
		DeleteContext: resourceMetricsAutoscalerDelete,

		Timeouts: &schema.ResourceTimeout{
			Create: schema.DefaultTimeout(12 * time.Minute),
			Update: schema.DefaultTimeout(12 * time.Minute),
			Delete: schema.DefaultTimeout(22 * time.Minute),
		},

		Schema: map[string]*schema.Schema{
			"metrics_instance_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"app_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"max_instances": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      10,
				ValidateFunc: validation.IntBetween(0, 1000000000),
			},
			"min_instances": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      1,
				ValidateFunc: validation.IntBetween(0, 1000000000),
			},
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"threshold_http_latency": {
				Type:     schema.TypeSet,
				Required: true,
				MaxItems: 1,
				Elem:     thresholdHTTPLatencySchema(),
			},
			"threshold_http_rate": {
				Type:     schema.TypeSet,
				Required: true,
				MaxItems: 1,
				Elem:     thresholdHTTPRateSchema(),
			},
			"threshold_memory": {
				Type:     schema.TypeSet,
				Required: true,
				MaxItems: 1,
				Elem:     thresholdMemorySchema(),
			},
			"threshold_cpu": {
				Type:     schema.TypeSet,
				Required: true,
				MaxItems: 1,
				Elem:     thresholdCPUSchema(),
			},
		},
	}
}

func thresholdHTTPLatencySchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"enabled": {
				Type:     schema.TypeBool,
				Required: true,
			},
			"max": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      10000,
				ValidateFunc: validation.FloatBetween(1, 1000000),
			},
			"min": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      10,
				ValidateFunc: validation.FloatBetween(1, 1000000),
			},
		},
	}
}

type threshold struct {
	fieldName string
	schema    func() *schema.Resource
}

var thresholdMapping = map[string]threshold{
	"cpu":          {"threshold_cpu", thresholdCPUSchema},
	"memory":       {"threshold_memory", thresholdMemorySchema},
	"http-rate":    {"threshold_http_rate", thresholdHTTPRateSchema},
	"http-latency": {"threshold_http_latency", thresholdHTTPLatencySchema},
}

func thresholdHTTPRateSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"enabled": {
				Type:     schema.TypeBool,
				Required: true,
			},
			"max": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      6000000,
				ValidateFunc: validation.FloatBetween(1, 6000000),
			},
			"min": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      300,
				ValidateFunc: validation.FloatBetween(1, 6000000),
			},
		},
	}
}

func thresholdMemorySchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"enabled": {
				Type:     schema.TypeBool,
				Required: true,
			},
			"max": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      100,
				ValidateFunc: validation.FloatBetween(0, 100),
			},
			"min": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      20,
				ValidateFunc: validation.FloatBetween(0, 100),
			},
		},
	}
}

func thresholdCPUSchema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"enabled": {
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"max": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      100,
				ValidateFunc: validation.FloatBetween(0, 100),
			},
			"min": {
				Type:         schema.TypeFloat,
				Optional:     true,
				Default:      5,
				ValidateFunc: validation.FloatBetween(0, 100),
			},
		},
	}
}

func resourceMetricsAutoscalerDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*config.Config)

	var diags diag.Diagnostics

	client, err := c.ConsoleClient()
	if err != nil {
		return diag.FromErr(err)
	}
	var app console.Application
	instanceID := d.Get("metrics_instance_id").(string)
	app.Name = d.Get("app_name").(string)
	app.Enabled = false
	result, err := updateWithRetry(client, instanceID, app)
	if err != nil {
		return diag.FromErr(err)
	}
	if result == nil {
		return diag.FromErr(fmt.Errorf("error creating/updating autoscaler"))
	}
	d.SetId("")
	return diags
}

func resourceMetricsAutoscalerUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	return resourceMetricsAutoscalerCreate(ctx, d, m)
}

func resourceMetricsAutoscalerRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*config.Config)

	var diags diag.Diagnostics

	client, err := c.ConsoleClient()
	if err != nil {
		return diag.FromErr(err)
	}
	instanceID := d.Get("metrics_instance_id").(string)
	name := d.Get("app_name").(string)

	app, err := getWitRetry(client, instanceID, name)
	if err != nil {
		return diag.FromErr(err)
	}
	_ = d.Set("min_instances", app.MinInstances)
	_ = d.Set("max_instances", app.MaxInstances)
	_ = d.Set("enabled", app.Enabled)
	for _, th := range app.Thresholds {
		mapping, ok := thresholdMapping[th.Name]
		if !ok {
			return diag.FromErr(fmt.Errorf("unknown threshold: %s", th.Name))
		}
		fields := make(map[string]interface{})
		fields["enabled"] = th.Enabled
		fields["min"] = th.Min
		fields["max"] = th.Max
		s := &schema.Set{F: schema.HashResource(mapping.schema())}
		s.Add(fields)
		_ = d.Set(mapping.fieldName, s)
	}
	return diags
}

func resourceMetricsAutoscalerCreate(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*config.Config)

	var diags diag.Diagnostics

	client, err := c.ConsoleClient()
	if err != nil {
		return diag.FromErr(err)
	}
	var app console.Application

	instanceID := d.Get("metrics_instance_id").(string)

	app.Name = d.Get("app_name").(string)
	app.MaxInstances = d.Get("max_instances").(int)
	app.MinInstances = d.Get("min_instances").(int)
	app.Enabled = d.Get("enabled").(bool)

	for key, mapping := range thresholdMapping {
		if v, ok := d.GetOk(mapping.fieldName); ok {
			vL := v.(*schema.Set).List()
			for _, vi := range vL {
				mVi := vi.(map[string]interface{})
				var threshold console.Threshold
				threshold.Name = key
				threshold.Min = mVi["min"].(float64)
				threshold.Max = mVi["max"].(float64)
				threshold.Enabled = mVi["enabled"].(bool)
				app.Thresholds = append(app.Thresholds, threshold)
			}
		}
	}
	created, err := updateWithRetry(client, instanceID, app)
	if err != nil {
		return diag.FromErr(err)
	}
	if created == nil {
		return diag.FromErr(fmt.Errorf("error creating/updating autoscaler"))
	}
	d.SetId(instanceID + created.Name)
	return diags
}

func updateWithRetry(client *console.Client, instanceID string, app console.Application) (*console.Application, error) {
	var created *console.Application
	operation := func() error {
		var err error
		var resp *console.Response
		created, resp, err = client.Metrics.UpdateApplicationAutoscaler(instanceID, app)
		return checkForIntermittentErrors(resp, err)
	}
	err := backoff.Retry(operation, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 30))
	return created, err
}

func getWitRetry(client *console.Client, instanceID string, name string) (*console.Application, error) {
	var app *console.Application
	operation := func() error {
		var err error
		var resp *console.Response
		app, resp, err = client.Metrics.GetApplicationAutoscaler(instanceID, name)
		return checkForIntermittentErrors(resp, err)
	}
	err := backoff.Retry(operation, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 30))
	return app, err
}

func checkForIntermittentErrors(resp *console.Response, err error) error {
	if resp == nil || resp.StatusCode > 500 {
		return err
	}
	if resp.StatusCode == http.StatusInternalServerError {
		return backoff.Permanent(fmt.Errorf("console: %s %w", resp.Error.Message, err))
	}
	if resp.StatusCode == http.StatusBadRequest &&
		strings.Contains(resp.Error.Message, "invalid character") {
		return config.ErrIntermittent
	}
	return backoff.Permanent(err)
}
