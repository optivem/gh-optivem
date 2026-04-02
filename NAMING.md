# Naming: System Name Replacements

During scaffolding, `gh optivem init` replaces the template system name ("Shop") with the user's `--system-name` across all source files, file names, and directories.

## Naming Derivation

Given `--system-name "Sky Travel"`:

| Casing | Template | Scaffolded | Used In |
|---|---|---|---|
| PascalCase | `Shop` | `SkyTravel` | Class names, .NET methods, file names |
| camelCase | `shop` | `skyTravel` | Java/TS methods, variables |
| kebab-case | `shop` | `sky-travel` | TS file names, URL routes |
| lowercase | `shop` | `skytravel` | Java package segments |

---

## Java Replacements

### Directories

| Template | Scaffolded |
|---|---|
| `dsl/core/usecase/shop/` | `dsl/core/usecase/skytravel/` |
| `dsl/driver/adapter/shop/` | `dsl/driver/adapter/skytravel/` |
| `dsl/driver/adapter/shop/api/` | `dsl/driver/adapter/skytravel/api/` |
| `dsl/driver/adapter/shop/api/client/` | `dsl/driver/adapter/skytravel/api/client/` |
| `dsl/driver/adapter/shop/api/client/controllers/` | `dsl/driver/adapter/skytravel/api/client/controllers/` |
| `dsl/driver/adapter/shop/api/client/dtos/` | `dsl/driver/adapter/skytravel/api/client/dtos/` |
| `dsl/driver/adapter/shop/ui/` | `dsl/driver/adapter/skytravel/ui/` |
| `dsl/driver/adapter/shop/ui/client/` | `dsl/driver/adapter/skytravel/ui/client/` |
| `dsl/driver/adapter/shop/ui/client/pages/` | `dsl/driver/adapter/skytravel/ui/client/pages/` |
| `dsl/driver/port/shop/` | `dsl/driver/port/skytravel/` |
| `dsl/driver/port/shop/dtos/` | `dsl/driver/port/skytravel/dtos/` |

### File Renames

| Template | Scaffolded |
|---|---|
| `ShopDsl.java` | `SkyTravelDsl.java` |
| `ShopDriver.java` | `SkyTravelDriver.java` |
| `ShopApiDriver.java` | `SkyTravelApiDriver.java` |
| `ShopUiDriver.java` | `SkyTravelUiDriver.java` |
| `ShopApiClient.java` | `SkyTravelApiClient.java` |
| `ShopUiClient.java` | `SkyTravelUiClient.java` |
| `BaseShopUseCase.java` | `BaseSkyTravelUseCase.java` |
| `GoToShop.java` | `GoToSkyTravel.java` |
| `ShopSmokeTest.java` | `SkyTravelSmokeTest.java` |
| `ShopApiSmokeTest.java` | `SkyTravelApiSmokeTest.java` |
| `ShopUiSmokeTest.java` | `SkyTravelUiSmokeTest.java` |
| `ShopBaseSmokeTest.java` | `SkyTravelBaseSmokeTest.java` |
| `ShopController.java` | `SkyTravelController.java` |

### Classes & Interfaces (content)

| Template | Scaffolded |
|---|---|
| `class ShopDsl` | `class SkyTravelDsl` |
| `interface ShopDriver` | `interface SkyTravelDriver` |
| `class ShopApiDriver implements ShopDriver` | `class SkyTravelApiDriver implements SkyTravelDriver` |
| `class ShopUiDriver implements ShopDriver` | `class SkyTravelUiDriver implements SkyTravelDriver` |
| `class ShopApiClient` | `class SkyTravelApiClient` |
| `class ShopUiClient` | `class SkyTravelUiClient` |
| `class BaseShopUseCase` | `class BaseSkyTravelUseCase` |
| `class GoToShop extends BaseShopUseCase` | `class GoToSkyTravel extends BaseSkyTravelUseCase` |
| `class ShopSmokeTest` | `class SkyTravelSmokeTest` |
| `class ShopBaseSmokeTest` | `class SkyTravelBaseSmokeTest` |
| `class ShopApiSmokeTest` | `class SkyTravelApiSmokeTest` |
| `class ShopUiSmokeTest` | `class SkyTravelUiSmokeTest` |
| `class ShopController` | `class SkyTravelController` |

### Methods (content)

| Template | Scaffolded |
|---|---|
| `goToShop()` | `goToSkyTravel()` |
| `shop()` | `skyTravel()` |
| `setUpShopBrowser()` | `setUpSkyTravelBrowser()` |
| `setUpShopHttpClient()` | `setUpSkyTravelHttpClient()` |
| `setUpShopUiClient()` | `setUpSkyTravelUiClient()` |
| `setUpShopApiClient()` | `setUpSkyTravelApiClient()` |
| `setUpShopUiDriver()` | `setUpSkyTravelUiDriver()` |
| `setUpShopApiDriver()` | `setUpSkyTravelApiDriver()` |
| `getShopApiBaseUrl()` | `getSkyTravelApiBaseUrl()` |
| `getShopUiBaseUrl()` | `getSkyTravelUiBaseUrl()` |
| `setShopDriver()` | `setSkyTravelDriver()` |
| `setShopClient()` | `setSkyTravelClient()` |
| `createShopDriver()` | `createSkyTravelDriver()` |
| `shouldBeAbleToGoToShop()` | `shouldBeAbleToGoToSkyTravel()` |

### Packages (content)

| Template | Scaffolded |
|---|---|
| `dsl.core.usecase.shop` | `dsl.core.usecase.skytravel` |
| `dsl.core.usecase.shop.commons` | `dsl.core.usecase.skytravel.commons` |
| `dsl.core.usecase.shop.usecases` | `dsl.core.usecase.skytravel.usecases` |
| `dsl.core.usecase.shop.usecases.base` | `dsl.core.usecase.skytravel.usecases.base` |
| `dsl.driver.adapter.shop` | `dsl.driver.adapter.skytravel` |
| `dsl.driver.adapter.shop.api` | `dsl.driver.adapter.skytravel.api` |
| `dsl.driver.adapter.shop.api.client` | `dsl.driver.adapter.skytravel.api.client` |
| `dsl.driver.adapter.shop.api.client.controllers` | `dsl.driver.adapter.skytravel.api.client.controllers` |
| `dsl.driver.adapter.shop.ui` | `dsl.driver.adapter.skytravel.ui` |
| `dsl.driver.adapter.shop.ui.client` | `dsl.driver.adapter.skytravel.ui.client` |
| `dsl.driver.adapter.shop.ui.client.pages` | `dsl.driver.adapter.skytravel.ui.client.pages` |
| `dsl.driver.port.shop` | `dsl.driver.port.skytravel` |
| `dsl.driver.port.shop.dtos` | `dsl.driver.port.skytravel.dtos` |

### Fields (content)

| Template | Scaffolded |
|---|---|
| `shopUiPlaywright` | `skyTravelUiPlaywright` |
| `shopUiBrowser` | `skyTravelUiBrowser` |
| `shopUiBrowserContext` | `skyTravelUiBrowserContext` |
| `shopUiPage` | `skyTravelUiPage` |
| `shopApiHttpClient` | `skyTravelApiHttpClient` |
| `shopUiClient` | `skyTravelUiClient` |
| `shopApiClient` | `skyTravelApiClient` |
| `shopDriver` | `skyTravelDriver` |

### String Literals (content)

| Template | Scaffolded |
|---|---|
| `"/shop"` | `"/sky-travel"` |
| `"a[href='/shop']"` | `"a[href='/sky-travel']"` |

---

## .NET Replacements

### Directories

| Template | Scaffolded |
|---|---|
| `Driver.Adapter/Shop/` | `Driver.Adapter/SkyTravel/` |
| `Driver.Adapter/Shop/Api/` | `Driver.Adapter/SkyTravel/Api/` |
| `Driver.Adapter/Shop/Ui/` | `Driver.Adapter/SkyTravel/Ui/` |
| `Driver.Port/Shop/` | `Driver.Port/SkyTravel/` |
| `Driver.Port/Shop/Dtos/` | `Driver.Port/SkyTravel/Dtos/` |
| `Dsl.Core/UseCase/Shop/` | `Dsl.Core/UseCase/SkyTravel/` |

### File Renames

| Template | Scaffolded |
|---|---|
| `ShopDsl.cs` | `SkyTravelDsl.cs` |
| `IShopDriver.cs` | `ISkyTravelDriver.cs` |
| `ShopApiDriver.cs` | `SkyTravelApiDriver.cs` |
| `ShopUiDriver.cs` | `SkyTravelUiDriver.cs` |
| `ShopApiClient.cs` | `SkyTravelApiClient.cs` |
| `ShopUiClient.cs` | `SkyTravelUiClient.cs` |
| `BaseShopCommand.cs` | `BaseSkyTravelCommand.cs` |
| `ShopUseCaseResult.cs` | `SkyTravelUseCaseResult.cs` |
| `GoToShop.cs` | `GoToSkyTravel.cs` |
| `WhenGoToShop.cs` | `WhenGoToSkyTravel.cs` |
| `IGoToShop.cs` | `IGoToSkyTravel.cs` |
| `Shop.csproj` | `SkyTravel.csproj` |
| `Shop.cshtml` | `SkyTravel.cshtml` |
| `Shop.cshtml.cs` | `SkyTravel.cshtml.cs` |
| `ShopSmokeTest.cs` | `SkyTravelSmokeTest.cs` |
| `ShopApiSmokeTest.cs` | `SkyTravelApiSmokeTest.cs` |
| `ShopUiSmokeTest.cs` | `SkyTravelUiSmokeTest.cs` |
| `ShopBaseSmokeTest.cs` | `SkyTravelBaseSmokeTest.cs` |
| `ShopModel.cs` (Shop.cshtml.cs) | `SkyTravelModel.cs` |

### Classes & Interfaces (content)

| Template | Scaffolded |
|---|---|
| `class ShopDsl` | `class SkyTravelDsl` |
| `interface IShopDriver` | `interface ISkyTravelDriver` |
| `class ShopApiDriver : IShopDriver` | `class SkyTravelApiDriver : ISkyTravelDriver` |
| `class ShopUiDriver : IShopDriver` | `class SkyTravelUiDriver : ISkyTravelDriver` |
| `class ShopApiClient` | `class SkyTravelApiClient` |
| `class ShopUiClient` | `class SkyTravelUiClient` |
| `class BaseShopCommand` | `class BaseSkyTravelCommand` |
| `class ShopUseCaseResult` | `class SkyTravelUseCaseResult` |
| `class GoToShop` | `class GoToSkyTravel` |
| `class WhenGoToShop` | `class WhenGoToSkyTravel` |
| `interface IGoToShop` | `interface IGoToSkyTravel` |
| `class ShopModel` | `class SkyTravelModel` |
| `class ShopSmokeTest` | `class SkyTravelSmokeTest` |
| `class ShopBaseSmokeTest` | `class SkyTravelBaseSmokeTest` |
| `class ShopApiSmokeTest` | `class SkyTravelApiSmokeTest` |
| `class ShopUiSmokeTest` | `class SkyTravelUiSmokeTest` |

### Methods (content)

| Template | Scaffolded |
|---|---|
| `GoToShopAsync()` | `GoToSkyTravelAsync()` |
| `Shop(Channel)` | `SkyTravel(Channel)` |
| `CreateShopDriverAsync()` | `CreateSkyTravelDriverAsync()` |
| `ShouldBeAbleToGoToShop()` | `ShouldBeAbleToGoToSkyTravel()` |

### Namespaces (content)

| Template | Scaffolded |
|---|---|
| `Driver.Adapter.Shop.Api` | `Driver.Adapter.SkyTravel.Api` |
| `Driver.Adapter.Shop.Api.Client` | `Driver.Adapter.SkyTravel.Api.Client` |
| `Driver.Adapter.Shop.Api.Client.Controllers` | `Driver.Adapter.SkyTravel.Api.Client.Controllers` |
| `Driver.Adapter.Shop.Api.Client.Dtos.Errors` | `Driver.Adapter.SkyTravel.Api.Client.Dtos.Errors` |
| `Driver.Adapter.Shop.Ui` | `Driver.Adapter.SkyTravel.Ui` |
| `Driver.Adapter.Shop.Ui.Client` | `Driver.Adapter.SkyTravel.Ui.Client` |
| `Driver.Adapter.Shop.Ui.Client.Pages` | `Driver.Adapter.SkyTravel.Ui.Client.Pages` |
| `Driver.Port.Shop` | `Driver.Port.SkyTravel` |
| `Driver.Port.Shop.Dtos` | `Driver.Port.SkyTravel.Dtos` |
| `Driver.Port.Shop.Dtos.Error` | `Driver.Port.SkyTravel.Dtos.Error` |
| `Dsl.Core.Shop` | `Dsl.Core.SkyTravel` |
| `Dsl.Core.Shop.UseCases` | `Dsl.Core.SkyTravel.UseCases` |
| `Dsl.Core.Shop.UseCases.Base` | `Dsl.Core.SkyTravel.UseCases.Base` |
| `Dsl.Port.When.Steps` | *(no change — "Shop" not in namespace)* |
| `Optivem.EShop.SystemTest.Core.Shop` | `Optivem.EShop.SystemTest.Core.SkyTravel` |

### Config Keys (content)

| Template | Scaffolded |
|---|---|
| `"Shop:UiBaseUrl"` | `"SkyTravel:UiBaseUrl"` |
| `"Shop:ApiBaseUrl"` | `"SkyTravel:ApiBaseUrl"` |

### Assembly Attributes (content)

| Template | Scaffolded |
|---|---|
| `"Shop"` (Company) | `"SkyTravel"` |
| `"Shop"` (Product) | `"SkyTravel"` |
| `"Shop"` (Title) | `"SkyTravel"` |

### String Literals (content)

| Template | Scaffolded |
|---|---|
| `"/shop"` (ShopButtonSelector) | `"/sky-travel"` |

---

## TypeScript Replacements

### Directories

| Template | Scaffolded |
|---|---|
| `src/app/shop/` | `src/app/sky-travel/` |

### File Renames

| Template | Scaffolded |
|---|---|
| `shop-api-driver.ts` | `sky-travel-api-driver.ts` |
| `shop-ui-driver.ts` | `sky-travel-ui-driver.ts` |
| `shop-smoke-test.spec.ts` | `sky-travel-smoke-test.spec.ts` |
| `shop-api-smoke-test.spec.ts` | `sky-travel-api-smoke-test.spec.ts` |
| `shop-ui-smoke-test.spec.ts` | `sky-travel-ui-smoke-test.spec.ts` |
| `Shop.tsx` | `SkyTravel.tsx` |

### Classes & Types (content)

| Template | Scaffolded |
|---|---|
| `ShopDriver` (interface) | `SkyTravelDriver` |
| `ShopApiDriver` (class) | `SkyTravelApiDriver` |
| `ShopUiDriver` (class) | `SkyTravelUiDriver` |
| `ShopPage` (component) | `SkyTravelPage` |
| `Shop` (component) | `SkyTravel` |

### Methods (content)

| Template | Scaffolded |
|---|---|
| `goToShop()` | `goToSkyTravel()` |
| `shop()` | `skyTravel()` |

### Variables (content)

| Template | Scaffolded |
|---|---|
| `shopDriver` | `skyTravelDriver` |

### Import Paths (content)

| Template | Scaffolded |
|---|---|
| `'./drivers/shop-api-driver'` | `'./drivers/sky-travel-api-driver'` |
| `'./drivers/shop-ui-driver'` | `'./drivers/sky-travel-ui-driver'` |

### Config Keys (content)

| Template | Scaffolded |
|---|---|
| `shop.frontendUrl` | `skyTravel.frontendUrl` |
| `shop.backendApiUrl` | `skyTravel.backendApiUrl` |
| `"shop"` (JSON key) | `"skyTravel"` |

### Route Paths (content)

| Template | Scaffolded |
|---|---|
| `"/shop"` (href, route) | `"/sky-travel"` |
| `"Shop Now"` (link text) | `"SkyTravel Now"` |
| `"Shop"` (breadcrumb) | `"Sky Travel"` |

### String Literals (content)

| Template | Scaffolded |
|---|---|
| `"Shop API not available: ..."` | `"Sky Travel API not available: ..."` |
| `"Shop UI not available: ..."` | `"Sky Travel UI not available: ..."` |
| `"Shop API Smoke Test"` | `"Sky Travel API Smoke Test"` |
| `"Shop UI Smoke Test"` | `"Sky Travel UI Smoke Test"` |
| `"Shop Smoke Test"` | `"Sky Travel Smoke Test"` |
| `"Shop"` (page title) | `"Sky Travel"` |

---

## Replacement Strategy

Replacements must be applied in order from **longest to shortest** to avoid partial matches:

### Pass 1: Multi-word compound names (PascalCase)

Replace longest patterns first:

1. `ShopBaseSmokeTest` → `SkyTravelBaseSmokeTest`
2. `ShopApiSmokeTest` → `SkyTravelApiSmokeTest`
3. `ShopUiSmokeTest` → `SkyTravelUiSmokeTest`
4. `ShopUseCaseResult` → `SkyTravelUseCaseResult`
5. `ShopSmokeTest` → `SkyTravelSmokeTest`
6. `ShopApiDriver` → `SkyTravelApiDriver`
7. `ShopUiDriver` → `SkyTravelUiDriver`
8. `ShopApiClient` → `SkyTravelApiClient`
9. `ShopUiClient` → `SkyTravelUiClient`
10. `BaseShopCommand` → `BaseSkyTravelCommand`
11. `BaseShopUseCase` → `BaseSkyTravelUseCase`
12. `IShopDriver` → `ISkyTravelDriver`
13. `IGoToShop` → `IGoToSkyTravel`
14. `GoToShop` → `GoToSkyTravel`
15. `ShopDriver` → `SkyTravelDriver`
16. `ShopModel` → `SkyTravelModel`
17. `ShopPage` → `SkyTravelPage`
18. `ShopDsl` → `SkyTravelDsl`

### Pass 2: Standalone `Shop` (PascalCase)

After compound names are handled, replace remaining standalone `Shop`:

- `Shop` → `SkyTravel` (class name, component name, titles)

### Pass 3: camelCase

- `goToShop` → `goToSkyTravel`
- `shopDriver` → `skyTravelDriver`
- `shopUi*` → `skyTravelUi*`
- `shopApi*` → `skyTravelApi*`
- `shop()` → `skyTravel()`

### Pass 4: kebab-case (file names, routes, imports)

- `shop-api-driver` → `sky-travel-api-driver`
- `shop-ui-driver` → `sky-travel-ui-driver`
- `shop-smoke-test` → `sky-travel-smoke-test`
- `shop-api-smoke-test` → `sky-travel-api-smoke-test`
- `shop-ui-smoke-test` → `sky-travel-ui-smoke-test`
- `/shop` → `/sky-travel`

### Pass 5: lowercase (Java packages)

- `.shop.` → `.skytravel.`
- `.shop` (end of package) → `.skytravel`

### Pass 6: File and directory renames

After all content replacements, rename files and directories using the same patterns.

---

## Collision Safety

The template name "Shop" is **not a substring** of any other identifier in the starter (no "Workshop", "ShopKeeper", etc.), so simple text replacement is safe. However, replacements must still be ordered longest-first to avoid double-replacing (e.g. replacing `Shop` inside an already-replaced `SkyTravelApiDriver`).

---

## System Name Validation

The `--system-name` value flows into multiple targets across all three languages. Each derived form must be valid in **every** context it appears:

| Derived Form | Targets | Constraints |
|---|---|---|
| PascalCase (`SkyTravel`) | Java class name, C# class name, C# namespace segment, TS class/type name, file name (all OS) | Must be valid identifier; no digits, hyphens, or special chars; not a reserved word; valid file name on Windows/Mac/Linux |
| camelCase (`skyTravel`) | Java method/field name, TS method/variable name, JSON key | Must be valid identifier; not a reserved word in any language |
| kebab-case (`sky-travel`) | TS file name, URL route path, Next.js directory name, import path | Must be valid URL segment; valid file/directory name on all OS |
| lowercase (`skytravel`) | Java package segment | Must be valid Java identifier; no hyphens; not a Java reserved word |

### Allowed

- Letters only: `a-z`, `A-Z`
- Spaces between words (used to derive casing variants)
- Minimum 1 character, maximum 50 characters
- Each word must start with a letter

### Rejected

| Rule | Reason (which target breaks) | Example |
|---|---|---|
| Empty or whitespace-only | All targets need a name | `""`, `"  "` |
| Starts with a digit | Java/C#/TS identifiers can't start with a digit | `"3D Print"` |
| Contains digits | Java package `skytravel3d` is valid but `3dskytravel` isn't; digits in class names reduce readability | `"Web3 App"` |
| Hyphens | Invalid in Java identifiers, Java packages, C# identifiers, C# namespaces | `"Sky-Travel"` |
| Underscores | Unconventional in PascalCase/camelCase class names; problematic in kebab-case file names | `"Sky_Travel"` |
| Dots | Conflicts with Java package separators and .NET namespace separators | `"Dr. Travel"` |
| Accented/unicode chars | Invalid or problematic in Java packages, file names, URL paths | `"Café"` |
| Special characters | Invalid in identifiers across all languages and in file names | `"Foo & Bar"`, `"My App!"` |
| Windows-illegal chars | `\ / : * ? " < > \|` are invalid in file/folder names | `"Q&A: Help"` |
| Leading/trailing spaces | Produces empty segments in kebab/package forms | `" Travel "` |
| Consecutive spaces | Produces double hyphens in kebab-case (`sky--travel`) | `"Sky  Travel"` |
| Single-char words | Produces unclear identifiers (`ATravel`, `aTravel`) | `"A Travel"` |
| Reserved words (any word) | Derived forms collide with language keywords | `"New"` → `new` (Java/C#/TS keyword) |
| Exceeds path limits | Windows MAX_PATH is 260; long names compound in deep paths | `"Extremely Long System Name That Goes On And On"` |

### Reserved Words to Check

Each **individual word** in the system name must not be a reserved keyword, because it may appear as a standalone identifier in derived forms (e.g. single-word system name). The **full derived forms** (PascalCase, camelCase, lowercase) must also not collide.

**Java** reserved words:
`abstract`, `assert`, `boolean`, `break`, `byte`, `case`, `catch`, `char`, `class`, `const`, `continue`, `default`, `do`, `double`, `else`, `enum`, `extends`, `final`, `finally`, `float`, `for`, `goto`, `if`, `implements`, `import`, `instanceof`, `int`, `interface`, `long`, `native`, `new`, `null`, `package`, `private`, `protected`, `public`, `return`, `short`, `static`, `strictfp`, `super`, `switch`, `synchronized`, `this`, `throw`, `throws`, `transient`, `try`, `void`, `volatile`, `while`

**C#** reserved words (additional to Java):
`as`, `base`, `bool`, `checked`, `decimal`, `delegate`, `event`, `explicit`, `extern`, `fixed`, `foreach`, `implicit`, `in`, `is`, `lock`, `namespace`, `object`, `operator`, `out`, `override`, `params`, `readonly`, `ref`, `sbyte`, `sealed`, `sizeof`, `stackalloc`, `string`, `struct`, `typeof`, `uint`, `ulong`, `unchecked`, `unsafe`, `ushort`, `using`, `virtual`, `where`, `yield`

**TypeScript** reserved words (additional to Java):
`any`, `async`, `await`, `constructor`, `declare`, `from`, `get`, `let`, `module`, `of`, `require`, `set`, `symbol`, `type`, `var`

### Validation Examples

| Input | Valid? | Reason |
|---|---|---|
| `Sky Travel` | Yes | Letters and space |
| `Pet Clinic` | Yes | Letters and space |
| `Todo` | Yes | Single word, letters only |
| `Book Store` | Yes | Letters and space |
| `A` | Yes | Minimum 1 letter |
| `Sky-Travel` | No | Hyphens invalid in Java identifiers/packages |
| `Sky_Travel` | No | Underscores unconventional in class names |
| `3D Print` | No | Starts with digit |
| `Web3 App` | No | Contains digit |
| `Café` | No | Accented character |
| `Dr. Travel` | No | Dot conflicts with package/namespace separators |
| `New` | No | `new` is reserved in Java/C#/TS |
| `Class Act` | No | `class` is reserved in Java/C#/TS |
| `For Real` | No | `for` is reserved in Java/C#/TS |
| `My App!` | No | Special character `!` |
| `A Travel` | No | Single-char word `A` |
| `Sky  Travel` | No | Consecutive spaces |
