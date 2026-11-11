package provider

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
				Description: "API client App-Token. Falls back to GLPI_APP_TOKEN. Required only when configured in GLPI.",
			},
			"purge_on_destroy": schema.BoolAttribute{
				Optional:    true,
				Description: "Permanently delete items on destroy instead of moving them to the GLPI trash bin. Falls back to GLPI_PURGE_ON_DESTROY=true.",
			},
			"user_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "User API token (\"Remote access key\" in user preferences). Falls back to GLPI_USER_TOKEN.",
			},
		},
	}
}

func orEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}

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
	rejectUnknown(&resp.Diagnostics, cfg)
	if resp.Diagnostics.HasError() {
		return
	}

	baseURL := resolveURL(&resp.Diagnostics, cfg.URL)
	user := orEnv(cfg.UserToken, "GLPI_USER_TOKEN")
	if user == "" {
		resp.Diagnostics.AddAttributeError(path.Root("user_token"), "Missing user token", "Set user_token or the GLPI_USER_TOKEN environment variable.")
	}
	purge := resolvePurge(&resp.Diagnostics, cfg.Purge)
	if resp.Diagnostics.HasError() {
		return
	}

	c := glpi.New(glpi.Config{BaseURL: baseURL, AppToken: orEnv(cfg.AppToken, "GLPI_APP_TOKEN"), UserToken: user, PurgeOnDestroy: purge})
	resp.ResourceData = c
	resp.DataSourceData = c
}

func rejectUnknown(diags *diag.Diagnostics, cfg providerModel) {
	for name, unknown := range map[string]bool{
		"url":              cfg.URL.IsUnknown(),
		"app_token":        cfg.AppToken.IsUnknown(),
		"user_token":       cfg.UserToken.IsUnknown(),
		"purge_on_destroy": cfg.Purge.IsUnknown(),
	} {
		if unknown {
			diags.AddAttributeError(path.Root(name), "Unknown value", "The value must be known at configuration time.")
		}
	}
}

func resolveURL(diags *diag.Diagnostics, configured types.String) string {
	baseURL := orEnv(configured, "GLPI_URL")
	if baseURL == "" {
		diags.AddAttributeError(path.Root("url"), "Missing GLPI URL", "Set url or the GLPI_URL environment variable.")
		return ""
	}
	if err := validateURL(baseURL); err != nil {
		diags.AddAttributeError(path.Root("url"), "Invalid GLPI URL", err.Error())
		return ""
	}
	if u, _ := url.Parse(baseURL); u.Scheme == "http" && !isLoopback(u.Hostname()) {
		diags.AddAttributeWarning(path.Root("url"), "Unencrypted GLPI URL", "The user token is sent over plain HTTP. Use https.")
	}
	return baseURL
}

func resolvePurge(diags *diag.Diagnostics, configured types.Bool) bool {
	if !configured.IsNull() {
		return configured.ValueBool()
	}
	env := os.Getenv("GLPI_PURGE_ON_DESTROY")
	if env == "" {
		return false
	}
	purge, err := strconv.ParseBool(env)
	if err != nil {
		diags.AddError("Invalid GLPI_PURGE_ON_DESTROY", err.Error())
	}
	return purge
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
