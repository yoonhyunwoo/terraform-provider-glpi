// Package provider implements the Terraform provider for GLPI.
package provider

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/yoonhyunwoo/terraform-provider-glpi/internal/glpi"
)

type glpiProvider struct{ version string }

type providerModel struct {
	URL       types.String `tfsdk:"url"`
	AppToken  types.String `tfsdk:"app_token"`
	UserToken types.String `tfsdk:"user_token"`
	Purge     types.Bool   `tfsdk:"purge_on_destroy"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &glpiProvider{version: version} }
}

func (p *glpiProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "glpi"
	resp.Version = p.version
}

func (p *glpiProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage GLPI assets through the GLPI legacy REST API (apirest.php).",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:    true,
				Description: "Base URL of the API, e.g. https://glpi.example.com/apirest.php. Falls back to GLPI_URL.",
			},
			"app_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API client App-Token. Falls back to GLPI_APP_TOKEN. Required only when the GLPI API client demands one.",
			},
			"purge_on_destroy": schema.BoolAttribute{
				Optional:    true,
				Description: "Permanently delete items on destroy instead of moving them to the GLPI trash bin (the default). Falls back to GLPI_PURGE_ON_DESTROY=true.",
			},
			"user_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "User API token (\"Remote access key\" in the user preferences). Falls back to GLPI_USER_TOKEN.",
			},
		},
	}
}

// orEnv prefers a non-empty configured value, so var.x defaulting to "" still falls back to the environment.
func orEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}

// validateURL never echoes the value, so credentials pasted into the URL cannot reach plan output.
func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("must be an absolute http(s) URL such as https://glpi.example.com/apirest.php")
	}
	if u.User != nil {
		return errors.New("must not contain credentials; use user_token")
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (p *glpiProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, unknown := range map[string]bool{
		"url":              cfg.URL.IsUnknown(),
		"app_token":        cfg.AppToken.IsUnknown(),
		"user_token":       cfg.UserToken.IsUnknown(),
		"purge_on_destroy": cfg.Purge.IsUnknown(),
	} {
		if unknown {
			resp.Diagnostics.AddAttributeError(path.Root(name), "Unknown value", "The value must be known at configuration time.")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	baseURL := orEnv(cfg.URL, "GLPI_URL")
	user := orEnv(cfg.UserToken, "GLPI_USER_TOKEN")
	if baseURL == "" {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "Missing GLPI URL", "Set url or the GLPI_URL environment variable.")
	} else if err := validateURL(baseURL); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("url"), "Invalid GLPI URL", err.Error())
	} else if u, _ := url.Parse(baseURL); u.Scheme == "http" && !isLoopback(u.Hostname()) {
		resp.Diagnostics.AddAttributeWarning(path.Root("url"), "Unencrypted GLPI URL", "The user token is sent over plain HTTP. Use https.")
	}
	if user == "" {
		resp.Diagnostics.AddAttributeError(path.Root("user_token"), "Missing user token", "Set user_token or the GLPI_USER_TOKEN environment variable.")
	}

	purge := cfg.Purge.ValueBool()
	if cfg.Purge.IsNull() {
		if env := os.Getenv("GLPI_PURGE_ON_DESTROY"); env != "" {
			b, err := strconv.ParseBool(env)
			if err != nil {
				resp.Diagnostics.AddError("Invalid GLPI_PURGE_ON_DESTROY", err.Error())
			}
			purge = b
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	c := glpi.New(glpi.Config{BaseURL: baseURL, AppToken: orEnv(cfg.AppToken, "GLPI_APP_TOKEN"), UserToken: user, PurgeOnDestroy: purge})
	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *glpiProvider) Resources(context.Context) []func() resource.Resource {
	out := make([]func() resource.Resource, 0, len(itemSpecs))
	for _, s := range itemSpecs {
		out = append(out, func() resource.Resource { return &itemResource{spec: s} })
	}
	return out
}

func (p *glpiProvider) DataSources(context.Context) []func() datasource.DataSource {
	out := make([]func() datasource.DataSource, 0, len(itemSpecs))
	for _, s := range itemSpecs {
		out = append(out, func() datasource.DataSource { return &itemDataSource{spec: s} })
	}
	return out
}
