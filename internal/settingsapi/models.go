package settingsapi

import (
	"slices"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

// ImageGenModel is one choice in Settings' Image generation picker.
type ImageGenModel struct {
	ID           string
	Name         string
	Provider     string
	ProviderName string
	Icon         string
	VendorPicks  bool
}

// ModelServed reports whether id names a model or routing group Magpie
// actually serves, including providers kept unlisted for routing groups.
func ModelServed(id string) bool {
	return slices.ContainsFunc(provider.Served(), func(e provider.Entry) bool {
		return e.ID == id || e.Group != "" && provider.SameRef(e.ID, id)
	})
}

// ImageGenModels is the list Settings' Image generation picker offers. It is
// also the list a newly chosen image model is checked against, so a chat-only
// model, a hidden drawer, or a group none of whose members draws is refused.
func ImageGenModels(current string) []ImageGenModel {
	out := []ImageGenModel{}
	for _, p := range provider.All() {
		if !p.On() || p.DecideOnly() {
			continue
		}
		// A hidden drawer stays visible only while it is the saved pick.
		for _, m := range gateway.Drawers(p) {
			if p.DrawerHidden(m.ID) && gateway.ListedDrawer(current) != p.ID+"/"+m.ID {
				continue
			}
			name := m.Name
			if name == "" {
				name = m.ID
			}
			out = append(out, ImageGenModel{
				ID:           p.ID + "/" + m.ID,
				Name:         name,
				Provider:     p.ID,
				ProviderName: p.Name,
				Icon:         p.Icon,
				VendorPicks:  gateway.VendorPicksDrawer(p),
			})
		}
	}
	// Routing groups whose members draw can be picked too, including one
	// whose image models are absent from the chat catalog.
	listed := map[string]bool{}
	for _, e := range provider.Served() {
		if e.Group != "" && gateway.GroupDraws(e.ID) {
			listed[e.ID] = true
			name := e.Name
			if name == "" {
				name = e.ID
			}
			out = append(out, ImageGenModel{ID: e.ID, Name: name, ProviderName: e.Group})
		}
	}
	for _, g := range provider.Groups() {
		ref := g.Ref()
		if g.Hidden || g.Disabled || listed[ref] || !gateway.GroupDraws(ref) {
			continue
		}
		name := g.Name
		if name == "" {
			name = g.ID
		}
		out = append(out, ImageGenModel{ID: ref, Name: name, ProviderName: g.ID})
	}
	return out
}

// ImageGenServed reports whether id is one of the current Image generation
// picker's choices. Saved values that disappear are kept by the caller; this
// check is only for a pick the user is making now.
func ImageGenServed(id string) bool {
	return slices.ContainsFunc(ImageGenModels(""), func(m ImageGenModel) bool {
		return m.ID == id
	})
}
