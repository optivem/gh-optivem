package actions

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

const javaGate = `    @EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS", matches = "1", disabledReason = "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run")`

func TestStripWipGate_Java(t *testing.T) {
	src := strings.Join([]string{
		"import org.junit.jupiter.api.TestTemplate;",
		"import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;",
		"",
		"class GiftWrapOrderPositiveTest extends BaseAcceptanceTest {",
		javaGate,
		"    @TestTemplate",
		"    @Channel({ChannelType.UI, ChannelType.API})",
		"    void shouldMarkOrderAsGiftWrapped() {",
		"    }",
		"",
		javaGate,
		"    @TestTemplate",
		"    @Channel({ChannelType.UI, ChannelType.API})",
		"    void shouldAddGiftWrappingFeeToOrderTotal() {",
		"    }",
		"}",
	}, "\n")

	t.Run("strips only the wanted test and keeps the import while a gate remains", func(t *testing.T) {
		out, stripped, found := StripWipGate(projectconfig.LangJava, src, map[string]bool{"shouldMarkOrderAsGiftWrapped": true})
		if !slices.Equal(stripped, []string{"shouldMarkOrderAsGiftWrapped"}) || !slices.Equal(found, []string{"shouldMarkOrderAsGiftWrapped"}) {
			t.Fatalf("stripped=%v found=%v", stripped, found)
		}
		if strings.Count(out, "@EnabledIfEnvironmentVariable(") != 1 {
			t.Fatalf("want exactly one gate left:\n%s", out)
		}
		if !strings.Contains(out, javaWipGateImport) {
			t.Fatalf("import removed while a gate is still used:\n%s", out)
		}
	})

	t.Run("strips all gates and the now-unused import", func(t *testing.T) {
		out, stripped, _ := StripWipGate(projectconfig.LangJava, src, map[string]bool{
			"shouldMarkOrderAsGiftWrapped": true, "shouldAddGiftWrappingFeeToOrderTotal": true,
		})
		if len(stripped) != 2 {
			t.Fatalf("stripped=%v", stripped)
		}
		if strings.Contains(out, "EnabledIfEnvironmentVariable") {
			t.Fatalf("gate or import left:\n%s", out)
		}
		if strings.Count(out, "@TestTemplate") != 2 || strings.Count(out, "@Channel(") != 2 {
			t.Fatalf("channel annotations must be kept:\n%s", out)
		}
	})

	t.Run("found but not gated is left unchanged", func(t *testing.T) {
		plain := "class X {\n    @TestTemplate\n    void shouldDoThing() {\n    }\n}"
		out, stripped, found := StripWipGate(projectconfig.LangJava, plain, map[string]bool{"shouldDoThing": true})
		if out != plain || len(stripped) != 0 || !slices.Equal(found, []string{"shouldDoThing"}) {
			t.Fatalf("out=%q stripped=%v found=%v", out, stripped, found)
		}
	})
}

func TestStripWipGate_Dotnet(t *testing.T) {
	src := strings.Join([]string{
		"public class GiftWrapOrderPositiveTest : BaseAcceptanceTest",
		"{",
		"    [SkippableTheory]",
		"    [ChannelData(ChannelType.UI, ChannelType.API)]",
		"    public async Task ShouldAddGiftWrappingFeeToOrderTotal(Channel channel)",
		"    {",
		`        Skip.IfNot(Environment.GetEnvironmentVariable("GH_OPTIVEM_RUN_WIP_TESTS") == "1", "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run");`,
		"        await Scenario(channel).When().PlaceOrder().Then().ShouldSucceed();",
		"    }",
		"}",
	}, "\r\n")
	out, stripped, _ := StripWipGate(projectconfig.LangDotnet, src, map[string]bool{"ShouldAddGiftWrappingFeeToOrderTotal": true})
	if !slices.Equal(stripped, []string{"ShouldAddGiftWrappingFeeToOrderTotal"}) {
		t.Fatalf("stripped=%v", stripped)
	}
	if strings.Contains(out, "Skip.IfNot") || strings.Contains(out, "SkippableTheory") {
		t.Fatalf("gate left:\n%s", out)
	}
	if !strings.Contains(out, "    [Theory]\r\n") || !strings.Contains(out, "[ChannelData(ChannelType.UI, ChannelType.API)]") {
		t.Fatalf("[Theory]/[ChannelData] not restored with CRLF intact:\n%q", out)
	}
}

func TestStripWipGate_Typescript(t *testing.T) {
	src := strings.Join([]string{
		"forChannels('ui', 'api')(() => {",
		"  test('shouldAddGiftWrappingFeeToOrderTotal', async ({ scenario }) => {",
		`    test.skip(process.env.GH_OPTIVEM_RUN_WIP_TESTS !== "1", "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run");`,
		"    await scenario.when().placeOrder().then().shouldSucceed();",
		"  });",
		"});",
	}, "\n")
	out, stripped, _ := StripWipGate(projectconfig.LangTypescript, src, map[string]bool{"shouldAddGiftWrappingFeeToOrderTotal": true})
	if len(stripped) != 1 || strings.Contains(out, "test.skip") {
		t.Fatalf("stripped=%v out:\n%s", stripped, out)
	}
	if !strings.Contains(out, "forChannels('ui', 'api')") {
		t.Fatalf("forChannels wrapper must be kept:\n%s", out)
	}
}

const tsGate = `        test.skip(process.env.GH_OPTIVEM_RUN_WIP_TESTS !== "1", "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run");`

func TestStripWipGate_TypescriptParameterised(t *testing.T) {
	cases := map[string]struct {
		src  []string
		want string
	}{
		"test.each, title on next line with $param suffix": {
			src: []string{
				"    test.each(nonExistentOrderCases)(",
				"        'shouldNotCancelNonExistentOrder_$orderNumber',",
				"        criteria('UC-003.6'),",
				"        async ({ scenario, orderNumber, message }) => {",
				tsGate,
				"            await scenario.when().cancelOrder();",
				"        },",
				"    );",
			},
			want: "shouldNotCancelNonExistentOrder",
		},
		"test.eachAlsoFirstRow with inline array": {
			src: []string{
				"    test.eachAlsoFirstRow(['3.5', 'lala'])(",
				"        'shouldRejectOrderWithNonIntegerQuantity_$quantity',",
				"        async ({ scenario, quantity }) => {",
				tsGate,
				"        },",
				"    );",
			},
			want: "shouldRejectOrderWithNonIntegerQuantity",
		},
		"test.each with object array, full template name reported": {
			src: []string{
				"    test.each([{ status: OrderStatus.CANCELLED }, { status: OrderStatus.DELIVERED }])(",
				"        'shouldNotDeliverOrder_$status',",
				"        async ({ scenario, status }) => {",
				"",
				tsGate,
				"        },",
				"    );",
			},
			want: "shouldNotDeliverOrder_$status",
		},
		"plain test with criteria before the callback": {
			src: []string{
				"    test(",
				"        'shouldPlaceOrder',",
				"        criteria('UC-001.1'),",
				"        async ({ scenario }) => {",
				tsGate,
				"        },",
				"    );",
			},
			want: "shouldPlaceOrder",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			src := strings.Join(tc.src, "\n")
			out, stripped, found := StripWipGate(projectconfig.LangTypescript, src, map[string]bool{tc.want: true})
			if len(found) != 1 || len(stripped) != 1 || strings.Contains(out, "test.skip") {
				t.Fatalf("found=%v stripped=%v out:\n%s", found, stripped, out)
			}
		})
	}
}

func TestStripWipGate_TypescriptGateOfNextTestUntouched(t *testing.T) {
	src := strings.Join([]string{
		"    test('shouldMine', async ({ scenario }) => {",
		"        await scenario.when().placeOrder();",
		"    });",
		"    test('shouldOther', async ({ scenario }) => {",
		tsGate,
		"    });",
	}, "\n")
	out, stripped, found := StripWipGate(projectconfig.LangTypescript, src, map[string]bool{"shouldMine": true})
	if out != src || len(stripped) != 0 || len(found) != 1 {
		t.Fatalf("another test's gate must stay: stripped=%v found=%v", stripped, found)
	}
}

func TestStripWipGate_UnwantedTestUntouched(t *testing.T) {
	src := "class X {\n" + javaGate + "\n    @TestTemplate\n    void shouldOther() {\n    }\n}"
	out, stripped, found := StripWipGate(projectconfig.LangJava, src, map[string]bool{"shouldMine": true})
	if out != src || len(stripped) != 0 || len(found) != 0 {
		t.Fatalf("an unlisted test's gate must stay: stripped=%v found=%v", stripped, found)
	}
}

func TestRemoveWipGate_Action(t *testing.T) {
	repo := t.TempDir()
	atDir := filepath.Join(repo, "system-test", "acceptance")
	if err := os.MkdirAll(atDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(atDir, "GiftWrapOrderPositiveTest.java")
	src := "import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;\nclass GiftWrapOrderPositiveTest {\n" + javaGate + "\n    @TestTemplate\n    void shouldAddGiftWrappingFeeToOrderTotal() {\n    }\n}\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &projectconfig.Config{}
	cfg.SystemTest.Lang = projectconfig.LangJava
	cfg.SystemTest.Paths = map[string]string{"at-test": filepath.Join("system-test", "acceptance")}
	a := actions{deps: Deps{Config: cfg, RepoPath: repo, Stdout: &bytes.Buffer{}}.withDefaults()}

	t.Run("strips the gate of the listed test", func(t *testing.T) {
		ctx := statemachine.NewContext()
		ctx.Set("test-names", "shouldAddGiftWrappingFeeToOrderTotal")
		if out := a.removeWipGate(ctx); out.Err != nil {
			t.Fatal(out.Err)
		}
		got, _ := os.ReadFile(file)
		if strings.Contains(string(got), "EnabledIfEnvironmentVariable") {
			t.Fatalf("gate or import left:\n%s", got)
		}
	})

	t.Run("fails loudly on a listed test that does not exist", func(t *testing.T) {
		ctx := statemachine.NewContext()
		ctx.Set("test-names", "shouldAddGiftWrappingFeeToOrderTotal, shouldNotExist")
		out := a.removeWipGate(ctx)
		if out.Err == nil || !strings.Contains(out.Err.Error(), "shouldNotExist") {
			t.Fatalf("want error naming shouldNotExist, got %v", out.Err)
		}
	})

	t.Run("fails loudly on empty test-names", func(t *testing.T) {
		if out := a.removeWipGate(statemachine.NewContext()); out.Err == nil {
			t.Fatal("want error on empty test-names")
		}
	})
}
