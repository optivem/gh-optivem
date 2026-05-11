# Naming: System Name Replacements

During scaffolding, `gh optivem init` replaces the template system name (`Shop`) with the value of `system_name:` in `gh-optivem.yaml` across all source files, file names, and directories. The yaml field is populated by `gh optivem config init --system-name …`.

## Input Format

`--system-name` (on `gh optivem config init`) accepts a **camelCase** name. Word boundaries are detected from uppercase transitions. No spaces needed.

```bash
gh optivem config init --system-name skyTravel ...
```

### Word Splitting Algorithm

Split on transitions between case boundaries. Consecutive uppercase letters are treated as an **acronym** (one word):

| Rule | Description |
|---|---|
| lowercase → uppercase | New word starts at the uppercase letter: `sky‖Travel` |
| Acronym → non-acronym | Consecutive uppercase followed by lowercase splits before the last uppercase: `ABC‖Store` |
| All lowercase | Single word: `todo` |
| All uppercase | Single word (acronym): `ABC` |

| Input | Words | Split Rule |
|---|---|---|
| `skyTravel` | `[sky, Travel]` | lowercase → uppercase |
| `eShop` | `[e, Shop]` | lowercase → uppercase |
| `eSuperStore` | `[e, Super, Store]` | lowercase → uppercase |
| `todo` | `[todo]` | no transition |
| `ABC` | `[ABC]` | all uppercase = acronym |
| `ABCStore` | `[ABC, Store]` | acronym → non-acronym |
| `myAPIClient` | `[my, API, Client]` | lowercase → acronym → non-acronym |

### Casing Derivation

Given `--system-name "skyTravel"`:

| Casing | Rule | Result | Used In |
|---|---|---|---|
| PascalCase | Capitalize first letter of each word, join | `SkyTravel` | Class names, .NET methods, file names |
| camelCase | Lowercase first word, capitalize rest, join | `skyTravel` | Java/TS methods, variables |
| kebab-case | Lowercase all words, join with `-` | `sky-travel` | TS file names, URL routes |
| lowercase | Lowercase all words, join | `skytravel` | Java package segments |

### More Examples

| Input | PascalCase | camelCase | kebab-case | lowercase |
|---|---|---|---|---|
| `skyTravel` | `SkyTravel` | `skyTravel` | `sky-travel` | `skytravel` |
| `eShop` | `EShop` | `eShop` | `e-shop` | `eshop` |
| `eSuperStore` | `ESuperStore` | `eSuperStore` | `e-super-store` | `esuperstore` |
| `todo` | `Todo` | `todo` | `todo` | `todo` |
| `petClinic` | `PetClinic` | `petClinic` | `pet-clinic` | `petclinic` |
| `bookStore` | `BookStore` | `bookStore` | `book-store` | `bookstore` |
| `ABC` | `ABC` | `aBC` | `abc` | `abc` |
| `ABCStore` | `ABCStore` | `aBCStore` | `abc-store` | `abcstore` |
| `myAPIClient` | `MyAPIClient` | `myAPIClient` | `my-api-client` | `myapiclient` |

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

The template name "Shop" is **not a substring** of any other identifier in the shop (no "Workshop", "ShopKeeper", etc.), so simple text replacement is safe. However, replacements must still be ordered longest-first to avoid double-replacing (e.g. replacing `Shop` inside an already-replaced `SkyTravelApiDriver`).

---

## System Name Validation

The `--system-name` value is provided in **camelCase** and flows into multiple targets across all three languages. Each derived form must be valid in **every** context it appears:

| Derived Form | Targets | Constraints |
|---|---|---|
| PascalCase (`SkyTravel`) | Java class name, C# class name, C# namespace segment, TS class/type name, file name (all OS) | Must be valid identifier; not a reserved word; valid file name on Windows/Mac/Linux |
| camelCase (`skyTravel`) | Java method/field name, TS method/variable name, JSON key | Must be valid identifier; not a reserved word in any language |
| kebab-case (`sky-travel`) | TS file name, URL route path, Next.js directory name, import path | Must be valid URL segment; valid file/directory name on all OS |
| lowercase (`skytravel`) | Java package segment | Must be valid Java identifier; no hyphens; not a Java reserved word |

### Allowed

- Letters only: `a-z`, `A-Z`
- camelCase format (word boundaries detected from case transitions)
- Minimum 1 character, maximum 50 characters

### Rejected

| Rule | Reason (which target breaks) | Example |
|---|---|---|
| Empty | All targets need a name | `""` |
| Leading/trailing spaces | Not valid camelCase | `" skyTravel"`, `"skyTravel "` |
| Contains digits | Java identifiers starting with digit are invalid; digits in class names reduce readability | `"web3App"` |
| Contains spaces | Input is camelCase, not space-separated | `"sky travel"` |
| Contains hyphens | Invalid in Java identifiers, Java packages, C# identifiers | `"sky-travel"` |
| Contains underscores | Unconventional in PascalCase/camelCase; problematic in kebab-case | `"sky_travel"` |
| Contains dots | Conflicts with Java package and .NET namespace separators | `"dr.travel"` |
| Accented/unicode chars | Invalid or problematic in Java packages, file names, URL paths | `"café"` |
| Special characters | Invalid in identifiers across all languages and in file names | `"foo&bar"` |
| Reserved words (language) | Derived forms collide with language keywords | `"new"` → `new` (keyword) |
| Reserved words (scaffold) | Collides with scaffolding infrastructure names (docker-compose, workflows, directory names) | `"Test System"` → `system` collides |
| Exceeds path limits | Windows MAX_PATH is 260; long names compound in deep paths | `"extremelyLongSystemNameThatGoesOnAndOn"` |

### Reserved Words to Check

Both the **full derived forms** (lowercase, camelCase) and each **individual word** (lowercased) must not collide with language keywords or scaffold infrastructure names.

#### Language Reserved Words

**Java** reserved words:
`abstract`, `assert`, `boolean`, `break`, `byte`, `case`, `catch`, `char`, `class`, `const`, `continue`, `default`, `do`, `double`, `else`, `enum`, `extends`, `final`, `finally`, `float`, `for`, `goto`, `if`, `implements`, `import`, `instanceof`, `int`, `interface`, `long`, `native`, `new`, `null`, `package`, `private`, `protected`, `public`, `return`, `short`, `static`, `strictfp`, `super`, `switch`, `synchronized`, `this`, `throw`, `throws`, `transient`, `try`, `void`, `volatile`, `while`

**C#** reserved words (additional to Java):
`as`, `base`, `bool`, `checked`, `decimal`, `delegate`, `event`, `explicit`, `extern`, `fixed`, `foreach`, `implicit`, `in`, `is`, `lock`, `namespace`, `object`, `operator`, `out`, `override`, `params`, `readonly`, `ref`, `sbyte`, `sealed`, `sizeof`, `stackalloc`, `string`, `struct`, `typeof`, `uint`, `ulong`, `unchecked`, `unsafe`, `ushort`, `using`, `virtual`, `where`, `yield`

**TypeScript** reserved words (additional to Java):
`any`, `async`, `await`, `constructor`, `declare`, `from`, `get`, `let`, `module`, `of`, `require`, `set`, `symbol`, `type`, `var`

#### Scaffold Reserved Words

These words appear in scaffolding infrastructure (docker-compose services, workflow files, directory names, Docker image names). If any **individual word** (lowercased) in the system name matches one of these, the replacement would corrupt infrastructure config.

`system`, `backend`, `frontend`, `test`, `api`, `external`, `stub`, `real`, `monolith`, `multitier`, `health`, `postgres`, `docker`, `compose`, `pipeline`, `local`, `stage`, `commit`, `acceptance`, `production`, `workflow`, `action`, `build`, `deploy`, `version`, `config`, `app`, `network`, `service`, `port`, `image`, `container`, `volume`, `env`, `run`, `src`, `main`, `lib`, `bin`, `dist`, `node`, `gradle`, `dotnet`, `java`, `typescript`, `react`, `spring`, `next`

### Validation Examples

| Input | Valid? | Reason |
|---|---|---|
| `skyTravel` | Yes | camelCase, letters only |
| `petClinic` | Yes | camelCase, letters only |
| `todo` | Yes | Single word, letters only |
| `bookStore` | Yes | camelCase, letters only |
| `eShop` | Yes | camelCase with single-char first word |
| `eSuperStore` | Yes | camelCase with single-char first word |
| `ABC` | Yes | All-uppercase acronym |
| `ABCStore` | Yes | Acronym + word |
| `myAPIClient` | Yes | Mixed acronym and words |
| `sky travel` | No | Spaces not allowed — use `skyTravel` |
| `sky-travel` | No | Hyphens not allowed — use `skyTravel` |
| `sky_travel` | No | Underscores not allowed — use `skyTravel` |
| `3dPrint` | No | Starts with digit |
| `web3App` | No | Contains digit |
| `café` | No | Accented character |
| `foo&bar` | No | Special character |
| `new` | No | `new` is a language reserved word |
| `newOrder` | No | `new` is a language reserved word |
| `classAct` | No | `class` is a language reserved word |
| `forReal` | No | `for` is a language reserved word |
| `testSystem` | No | `test` and `system` are scaffold reserved words |
| `backendApi` | No | `backend` and `api` are scaffold reserved words |
| `frontendApp` | No | `frontend` and `app` are scaffold reserved words |
