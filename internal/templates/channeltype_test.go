package templates

import "testing"

// The full api+ui set in declaration order must render the same constants the
// hand-maintained shop copies carried (idiomatic uppercase name and value),
// per language. Placeholders in the namespace/package are left intact for the
// later Replace* passes.
func TestRenderChannelType_FullSet(t *testing.T) {
	channels := []string{"api", "ui"}

	wantCSharp := `namespace MyCompany.MyShop.SystemTest.Channel;

public static class ChannelType
{
    public const string API = "API";
    public const string UI = "UI";
}
`
	if got := renderChannelTypeCSharp(channels); got != wantCSharp {
		t.Errorf("C# render mismatch:\ngot:\n%s\nwant:\n%s", got, wantCSharp)
	}

	wantJava := `package com.mycompany.myshop.testkit.channel;

public class ChannelType {
    public static final String API = "API";
    public static final String UI = "UI";

    private ChannelType() {
        // Utility class
    }
}
`
	if got := renderChannelTypeJava(channels); got != wantJava {
		t.Errorf("Java render mismatch:\ngot:\n%s\nwant:\n%s", got, wantJava)
	}

	wantTS := `export const ChannelType = {
    API: 'API',
    UI: 'UI',
} as const;

export type ChannelTypeValue = (typeof ChannelType)[keyof typeof ChannelType];
`
	if got := renderChannelTypeTypeScript(channels); got != wantTS {
		t.Errorf("TS render mismatch:\ngot:\n%s\nwant:\n%s", got, wantTS)
	}
}

// A subset (API-only project) must render only the declared channels — proving
// the constant set is driven by channels:, not a fixed verbatim copy.
func TestRenderChannelType_Subset(t *testing.T) {
	channels := []string{"api"}

	wantCSharp := `namespace MyCompany.MyShop.SystemTest.Channel;

public static class ChannelType
{
    public const string API = "API";
}
`
	if got := renderChannelTypeCSharp(channels); got != wantCSharp {
		t.Errorf("C# subset render mismatch:\ngot:\n%s\nwant:\n%s", got, wantCSharp)
	}

	wantTS := `export const ChannelType = {
    API: 'API',
} as const;

export type ChannelTypeValue = (typeof ChannelType)[keyof typeof ChannelType];
`
	if got := renderChannelTypeTypeScript(channels); got != wantTS {
		t.Errorf("TS subset render mismatch:\ngot:\n%s\nwant:\n%s", got, wantTS)
	}
}
