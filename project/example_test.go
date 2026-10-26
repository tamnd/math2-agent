package project_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/project"
)

func ExampleProject_Allows() {
	p := &project.Project{ID: "fermat", Roles: []string{"explore"}, Members: []project.Person{{ID: "github:octocat", Role: project.Owner}}}
	fmt.Println(p.Allows(&agent.Job{Role: "explore", OnBehalfOf: "project:fermat", RequestedBy: "github:octocat"}))
	fmt.Println(p.Allows(&agent.Job{Role: "prove", OnBehalfOf: "project:fermat"}))
	// Output:
	// <nil>
	// project fermat: the role prove does not work for the project. Its roles are explore
}
