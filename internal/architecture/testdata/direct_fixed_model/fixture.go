package directfixedmodel

import (
	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
)

func decision() router.Decision {
	return router.Decision{Provider: providers.ProviderAIAND, Model: "moonshotai/kimi-k3"}
}
