package customers

// Feature keys for Control proxy gates.
const (
	FeatureControl  = "control"
	FeaturePresence = "presence"
	FeatureHistory  = "history"
	FeatureInject   = "inject"
	FeatureModerate = "moderate"
	FeatureRecord   = "record"
	FeatureSessions = "sessions"
	FeatureFleet    = "fleet"
)

// DefaultControlFeatures enables all Control proxy features (seed for acme).
func DefaultControlFeatures() map[string]any {
	return map[string]any{
		FeatureControl:  true,
		FeaturePresence: true,
		FeatureHistory:  true,
		FeatureInject:   true,
		FeatureModerate: true,
		FeatureRecord:   true,
		FeatureSessions: true,
		FeatureFleet:    true,
	}
}

// FeatureEnabled reports whether features[key] is boolean true.
func FeatureEnabled(features map[string]any, key string) bool {
	if features == nil {
		return false
	}
	v, ok := features[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

// ControlAllowed reports whether master control and the per-action feature are on.
func ControlAllowed(features map[string]any, action string) bool {
	return FeatureEnabled(features, FeatureControl) && FeatureEnabled(features, action)
}

// MergeFeatures copies base and overlays patch keys.
func MergeFeatures(base, patch map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

// Patch updates status and/or merges features for an existing customer.
func (d *Directory) Patch(id string, status *string, features map[string]any) (Customer, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	c, ok := d.byID[id]
	if !ok {
		return Customer{}, false
	}
	if status != nil {
		c.Status = *status
	}
	if features != nil {
		c.Features = MergeFeatures(c.Features, features)
	}
	return clone(*c), true
}
