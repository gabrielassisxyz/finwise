package main

import (
	"fmt"

	"github.com/gabrielassisxyz/finwise/internal/api"
)

const (
	agentTokenUsage        = "usage: finwise agent-token"
	agentTokenVariable     = "FINWISE_AGENT_TOKEN"
	agentTokenHashVariable = "FINWISE_AGENT_TOKEN_HASH"
)

// runAgentToken makes the one credential an agent reads with and prints it
// with its hash, as the two lines the person pastes into the agent's
// environment and the server's. The token is shown because, unlike a PIN, the
// person cannot know it beforehand; it is written nowhere else, and nothing
// else is printed.
func runAgentToken(args []string, env environment) int {
	if len(args) != 0 {
		fmt.Fprintln(env.stderr, agentTokenUsage)
		return exitUsage
	}
	token, stored, err := api.NewAgentToken()
	if err != nil {
		fmt.Fprintln(env.stderr, "finwise: could not make a token")
		return exitFailure
	}
	fmt.Fprintf(env.stdout, "%s=%s\n%s=%s\n", agentTokenVariable, token, agentTokenHashVariable, stored)
	return 0
}
