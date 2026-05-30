package templates

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/log"
)

// ChannelType source is generated from the project's declared channel set
// (gh-optivem.yaml channels:) rather than copied verbatim, so the constant set
// has a single source of truth instead of three hand-maintained per-language
// copies. Each lowercase channels: token (api) becomes an uppercase constant
// name and value (API = "API") — the testkit-internal channel value the drivers
// switch against. The namespace/package keeps the MyCompany / MyShop
// placeholders the later Replace* passes rewrite per project, so generation
// must run before those passes (it does — ApplyTemplate precedes them).

// channelTypeTarget locates the ChannelType source under system-test/ for a
// test language and renders its content from the channel token set.
type channelTypeTarget struct {
	relPath string
	render  func(channels []string) string
}

var channelTypeTargets = map[string]channelTypeTarget{
	"dotnet": {
		relPath: filepath.Join("Channel", "ChannelType.cs"),
		render:  renderChannelTypeCSharp,
	},
	"java": {
		relPath: filepath.Join("src", "main", "java", "com", "mycompany", "myshop", "testkit", "channel", "ChannelType.java"),
		render:  renderChannelTypeJava,
	},
	"typescript": {
		relPath: filepath.Join("src", "testkit", "channel", "channel-type.ts"),
		render:  renderChannelTypeTypeScript,
	},
}

// WriteChannelType regenerates the testkit's ChannelType source from the
// declared channel set, overwriting the verbatim copy ApplyTemplate placed in
// systemTestDir. testLang selects the language form; channels are the lowercase
// canonical tokens (api, ui) in declaration order.
func WriteChannelType(systemTestDir, testLang string, channels []string) {
	target, ok := channelTypeTargets[testLang]
	if !ok {
		log.Warnf("ChannelType codegen: unknown test language %q, skipping", testLang)
		return
	}
	path := filepath.Join(systemTestDir, target.relPath)
	if err := os.WriteFile(path, []byte(target.render(channels)), 0644); err != nil {
		log.Warnf("ChannelType codegen: failed to write %s: %v", path, err)
		return
	}
	log.Successf("Generated ChannelType from channels %v (%s)", channels, testLang)
}

// channelConst is the uppercase constant name and value for a lowercase token.
func channelConst(token string) string {
	return strings.ToUpper(token)
}

func renderChannelTypeCSharp(channels []string) string {
	var b strings.Builder
	b.WriteString("namespace MyCompany.MyShop.SystemTest.Channel;\n\n")
	b.WriteString("public static class ChannelType\n{\n")
	for _, ch := range channels {
		c := channelConst(ch)
		b.WriteString("    public const string " + c + " = \"" + c + "\";\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderChannelTypeJava(channels []string) string {
	var b strings.Builder
	b.WriteString("package com.mycompany.myshop.testkit.channel;\n\n")
	b.WriteString("public class ChannelType {\n")
	for _, ch := range channels {
		c := channelConst(ch)
		b.WriteString("    public static final String " + c + " = \"" + c + "\";\n")
	}
	b.WriteString("\n    private ChannelType() {\n        // Utility class\n    }\n}\n")
	return b.String()
}

func renderChannelTypeTypeScript(channels []string) string {
	var b strings.Builder
	b.WriteString("export const ChannelType = {\n")
	for _, ch := range channels {
		c := channelConst(ch)
		b.WriteString("    " + c + ": '" + c + "',\n")
	}
	b.WriteString("} as const;\n\n")
	b.WriteString("export type ChannelTypeValue = (typeof ChannelType)[keyof typeof ChannelType];\n")
	return b.String()
}
