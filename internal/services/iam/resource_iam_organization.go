package iam

import (
	"context"
	"fmt"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/philips-software/go-hsdp-api/iam"
	"github.com/philips-software/terraform-provider-hsdp/internal/config"
	"github.com/philips-software/terraform-provider-hsdp/internal/tools"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func ResourceIAMOrg() *schema.Resource {
	return &schema.Resource{
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		SchemaVersion: 3,
		CreateContext: resourceIAMOrgCreate,
		ReadContext:   resourceIAMOrgRead,
		UpdateContext: resourceIAMOrgUpdate,
		DeleteContext: resourceIAMOrgDelete,

		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				ForceNew: true,
				Required: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"display_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"type": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"external_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"is_root_org": {
				Type:       schema.TypeBool,
				Optional:   true,
				Deprecated: "This field is deprecated, please remove it",
			},
			"parent_org_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"active": {
				Type:     schema.TypeBool,
				Computed: true,
			},
		},
	}
}

func resourceIAMOrgCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*config.Config)

	client, err := c.IAMClient()
	if err != nil {
		return diag.FromErr(err)
	}

	name := d.Get("name").(string)
	description := d.Get("description").(string)
	externalID := d.Get("external_id").(string)
	orgType := d.Get("type").(string)
	displayName := d.Get("display_name").(string)
	parentOrgID, ok := d.Get("parent_org_id").(string)
	if !ok {
		return diag.FromErr(config.ErrMissingParentOrgID)
	}
	var newOrg iam.Organization
	newOrg.Name = name
	newOrg.Description = description
	newOrg.Parent.Value = parentOrgID
	newOrg.ExternalID = externalID
	newOrg.DisplayName = displayName
	newOrg.Type = orgType

	var org *iam.Organization
	var resp *iam.Response
	err = tools.TryHTTPCall(ctx, 10, func() (*http.Response, error) {
		var err error
		org, resp, err = client.Organizations.CreateOrganization(newOrg)
		if err != nil {
			_ = client.TokenRefresh()
		}
		if resp == nil {
			return nil, err
		}
		return resp.Response, err
	})
	if err != nil {
		return diag.FromErr(err)
	}
	if org == nil {
		return diag.FromErr(fmt.Errorf("failed to create organization: %d", resp.StatusCode))
	}
	d.SetId(org.ID)
	return resourceIAMOrgRead(ctx, d, m)
}

func resourceIAMOrgRead(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*config.Config)

	var diags diag.Diagnostics

	client, err := c.IAMClient()
	if err != nil {
		return diag.FromErr(err)
	}

	id := d.Id()
	org, resp, err := client.Organizations.GetOrganizationByID(id)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			d.SetId("")
			return nil
		}
		return diag.FromErr(err)
	}
	_ = d.Set("description", org.Description)
	_ = d.Set("name", org.Name)
	_ = d.Set("external_id", org.ExternalID)
	_ = d.Set("parent_org_id", org.Parent.Value)
	_ = d.Set("display_name", org.DisplayName)
	_ = d.Set("active", org.Active)
	_ = d.Set("type", org.Type)
	return diags
}

func resourceIAMOrgUpdate(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*config.Config)

	var diags diag.Diagnostics

	client, err := c.IAMClient()
	if err != nil {
		return diag.FromErr(err)
	}

	id := d.Id()
	org, _, err := client.Organizations.GetOrganizationByID(id)
	if err != nil {
		return diag.FromErr(err)
	}

	if d.HasChange("description") || d.HasChange("name") ||
		d.HasChange("type") || d.HasChange("external_id") || d.HasChange("display_name") {
		org.Name = d.Get("name").(string)
		org.Description = d.Get("description").(string)
		org.Type = d.Get("type").(string)
		org.ExternalID = d.Get("external_id").(string)
		org.DisplayName = d.Get("display_name").(string)
		_, _, err = client.Organizations.UpdateOrganization(*org)
		if err != nil {
			diags = append(diags, diag.FromErr(err)...)
		}
	}
	return diags
}

func resourceIAMOrgDelete(_ context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*config.Config)

	var diags diag.Diagnostics

	client, err := c.IAMClient()
	if err != nil {
		return diag.FromErr(err)
	}

	id := d.Id()
	org, _, err := client.Organizations.GetOrganizationByID(id)
	if err != nil {
		return diag.FromErr(err)
	}

	ok, _, err := client.Organizations.DeleteOrganization(*org)
	if err != nil {
		return diag.FromErr(err)
	}
	if !ok {
		return diag.FromErr(config.ErrInvalidResponse)
	}
	return diags
}
