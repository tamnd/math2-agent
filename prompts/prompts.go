// Package prompts holds the prompts of the agent roles, each at its role version (spec 12.8).
//
// [FS] has one directory for each role that is written. A directory has role.yaml with the version, the tool profile, the escalation ladder, the default budget and the limits of a run, system.md with the system prompt, style/ with the conventions that the model gets after the system prompt, checks/ with the prompts of the checks that the goal runs with a model, and CHANGELOG.md. agent.LoadRole reads a role from FS.
//
// A change to any file of a role bumps the version in its role.yaml, and the CHANGELOG.md of the role tells why. The file roles.sum has the version and the digest of the files of each role, and the tests fail when the files change and the version does not go up. The prompts have no text from a source and no instructions from a node: that text comes only in the results of the tools, as data.
package prompts

import "embed"

// FS holds the directory of each role that is written.
//
//go:embed align bump curate enumerate explain explore formalize prove review-assist
var FS embed.FS
