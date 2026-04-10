# Kaivue Grafana Dashboards

Three pre-built dashboards ship with Kaivue. All dashboards:

- Use Grafana 10 JSON format with the `__inputs` / `__requires` envelope.
- Require a Prometheus data source named `DS_PROMETHEUS`.
- Contain no brand logos or colours (white-label seam — apply branding via Grafana themes or Grafana Organizations in v1.x).

## Dashboards

| File | Audience | UID |
|---|---|---|
| `customer-admin.json` | Customer administrator | `kaivue-customer-admin` |
| `integrator-portal.json` | Integrator / reseller | `kaivue-integrator-portal` |
| `internal-on-call.json` | Kaivue SRE on-call | `kaivue-internal-on-call` |

## Importing

### Grafana UI

1. Open Grafana and navigate to **Dashboards → Import**.
2. Upload the JSON file or paste its contents.
3. Select your Prometheus data source when prompted for `DS_PROMETHEUS`.
4. Click **Import**.

### Grafana provisioning (recommended for on-prem installs)

Place the JSON files in your Grafana provisioning directory and add a provider:

```yaml
# /etc/grafana/provisioning/dashboards/kaivue.yaml
apiVersion: 1
providers:
  - name: kaivue
    type: file
    options:
      path: /etc/grafana/dashboards/kaivue
```

Copy the three JSON files to `/etc/grafana/dashboards/kaivue/` and restart Grafana.

## Scrape configuration

See `docs/operations/metrics.md` for how to point Prometheus at the Kaivue `/metrics` endpoint.

## White-label note

Branding (colours, logo) must not be embedded in dashboard JSON. Apply
per-integrator styling through Grafana Organizations or Grafana's theming API
in v1.x. This keeps the provisioned JSON files re-usable across all
integrators without modification.
