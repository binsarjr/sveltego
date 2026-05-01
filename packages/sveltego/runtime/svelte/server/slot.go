package server

// SlotFn is the function signature compiled slots use: receives the
// current payload and the slot's locally-scoped props.
type SlotFn func(p *Payload, slotProps map[string]any)

// Slot renders a slot from props["$$slots"][name] / props[name], with a
// fallback when no slot is provided. Mirrors svelte/internal/server.slot.
//
// Compiled output passes the slot function through props; if absent, the
// fallback fn renders. fallback may be nil for slots with no default.
func Slot(p *Payload, props map[string]any, name string, slotProps map[string]any, fallback func(*Payload)) {
	if fn := lookupSlot(props, name); fn != nil {
		fn(p, slotProps)
		return
	}
	if fallback != nil {
		fallback(p)
	}
}

func lookupSlot(props map[string]any, name string) SlotFn {
	if slots, ok := props["$$slots"].(map[string]any); ok {
		if raw, ok := slots[name]; ok {
			if fn, ok := raw.(SlotFn); ok {
				return fn
			}
			if fn, ok := raw.(func(*Payload, map[string]any)); ok {
				return fn
			}
		}
	}
	key := name
	if name == "default" {
		key = "children"
	}
	if raw, ok := props[key]; ok {
		if fn, ok := raw.(SlotFn); ok {
			return fn
		}
		if fn, ok := raw.(func(*Payload, map[string]any)); ok {
			return fn
		}
	}
	return nil
}
