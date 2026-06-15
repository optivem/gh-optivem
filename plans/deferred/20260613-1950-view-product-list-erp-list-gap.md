# Plan: close the ERP product-list staging gap behind #65 "View product list" ({20260613-1950})

⏳ **PENDING — awaiting operator confirmation.** Not yet picked up. This file
records the #65 root-cause investigation and the proposed fix; do not start
`## Items` until the operator confirms the chosen option (see "Decision needed").

Context: the `atdd-rehearsal-loop.sh` corpus run on 2026-06-11 stopped at #65.
The `system-implementer` (api, Java monolith) emitted a **scope-exception** and
refused to write production code. Investigation below confirms the refusal was
correct and the fix lives **upstream of** `system-implementer`, in the test-kit.

---

## Why stub `/products` if "no one calls it"? (the caller is production)

A natural objection: today nothing calls `GET /erp/api/products`, so why stub it?
Because **production calls it** — that's the whole feature. Walk the #65 chain:

```
AT: "shouldListAllProductsWhenProductsAreAvailable"
   │
   ▼
HTTP GET /api/products        ← test hits MyShop's public API
   │
   ▼
ProductApiController          (item 5 — new)
   │
   ▼
ErpGateway.getProducts()      (item 4 — new) ──calls──►  GET /erp/api/products   ← HERE
   │
   ▼
returns the list → rendered back to the test
```

The new production code (`ErpGateway.getProducts()`, items 4–5) issues `GET
/erp/api/products`. It's the only way MyShop can answer "list all products":
MyShop owns no products table (ERP owns the catalog; MyShop proxies). So the
stub exists **because** production calls `/products` — no stub → that real
production call hits an unstubbed endpoint → 404 → test fails.

The framing is inverted from the objection:
- **Not** "stub `/products` even though no one calls it."
- But "**production calls `/products`** to fetch the list, therefore the test
  must stub `/products` to back that call."

This is exactly why the per-SKU style fails: it stubs `/products/A`,
`/products/B`, but production's lister never calls those — it calls `/products`,
which the per-SKU style leaves unstubbed. "No one calls `/products`" only *feels*
true today because production has no lister yet; #65 is the story that
**introduces** the caller. Stub and caller are added together red→green: stub
`/products` (items 1–3) so the new production caller (items 4–5) has something to
talk to.

---

## TL;DR

**Why:** "View product list" needs an *enumerable* product source. The real ERP
simulator (`shop/external-systems/simulators/mock-server.js`) already exposes
`GET /erp/api/products` (a full list, via json-server) — and since ERP is
`real-kind: simulator` for `gh-optivem-monolith-java.yaml`, that list endpoint
**is** the real ERP contract. But the test-kit only ever built the **per-SKU**
path (`given().product()` → `erp().returnsProduct()` → stub `GET
/erp/api/products/{sku}`). The rehearsal's acceptance-test-writer + dsl-implementer
staged the new AT on that per-SKU primitive, which **production cannot
enumerate**. By the time `system-implementer` ran, the test design was frozen and
no in-scope change under `system/monolith/java` + `system/db/migrations` could
green `shouldListAllProductsWhenProductsAreAvailable`. Hence the (correct)
scope-exception → `STOP_SCOPE_VIOLATION` crash.

**End result:** an enumerable ERP product-list path exists end-to-end (stub →
contract → DSL → production `ErpGateway.getProducts()` → `ProductApiController`
`GET /api/products`), so #65 is a clean read-only **proxy** story that greens
without a MyShop-owned products table.

---

## Verdict: genuine gap, refusal correct

The `system-implementer` refusal was **not** a dodge:

- It correctly refused — no change confined to `system/monolith/java` +
  `system/db/migrations` can enumerate per-SKU WireMock stubs.
- It correctly named the fix — "an enumerable ERP product-list stub + contract so
  production can proxy `GET /erp/api/products`."
- It correctly avoided adding a `products` migration — products are ERP-owned;
  MyShop proxies, it does not own a catalog.

**Correction to the agent's own report:** it offered a second option — a
MyShop-owned catalog seeded by the DSL. That option is **wrong** for this SUT:
the real ERP owns products, so MyShop must proxy. Only the enumerable-ERP-stub
option is right.

## The actual gap (per layer)

| Layer | Exists today | Missing for a list |
|---|---|---|
| Real ERP (simulator) | `GET /erp/api/products` **and** `/{sku}` | — (already there) |
| ERP **stub** (`ErpStubClient`/`ErpStubDriver`) | stubs `/{sku}` only | no `GET /erp/api/products` list stub |
| DSL (`given().product()` → `erp().returnsProduct()`) | one per-SKU stub | no enumerable `given().products(...)` |
| ERP contract test | `shouldBeAbleToGetProduct()` (singular) | no list contract |
| Production `ErpGateway` | `getProductDetails(sku)` | no `getProducts()` |
| Production API | Order / Coupon controllers | no `ProductApiController` / `GET /api/products` |

Key files (all under `C:\Users\valen\Documents\GitHub\optivem\academy\shop`):
- `system-test/java/.../dsl/core/scenario/given/steps/GivenProductImpl.java`
- `system-test/java/.../driver/adapter/external/erp/client/{BaseErpClient,ErpStubClient}.java`
- `system-test/java/.../driver/adapter/external/erp/{ErpStubDriver,BaseErpDriver}.java`
- `system-test/java/.../driver/port/external/erp/ErpDriver.java`
- `system-test/java/src/test/.../contract/erp/BaseErpContractTest.java`
- `system/monolith/java/.../core/services/external/ErpGateway.java`
- `system/monolith/java/.../api/controller/` (add `ProductApiController`)
- `external-systems/simulators/mock-server.js` (already serves the list — no change)

---

## Decision needed (pick before execution)

**Option A — build the enumerable ERP list path in `shop` (recommended).**
Matches the real ERP contract; makes #65 implementable end-to-end. Larger change
spanning `system-test` + `monolith`. Items below assume this option.

**Option B — fix orchestrator guidance only (`gh-optivem`).** Leave `shop` alone;
update the `acceptance-test-writer` / `dsl-implementer` agent prompts so
list-shaped, externally-backed stories stage an enumerable source instead of
reusing per-SKU `given().product()`. Cheaper, but #65 still can't green until the
test-kit list path exists, so this is complementary to A, not a substitute.

**Option C — reframe #65 in the corpus.** Treat #65 as a poor rehearsal fit and
drop/annotate it in `DEFAULT_TICKETS` + `CONTRIBUTING.md`. Lowest effort; abandons
a legitimate read-only story the real ERP already supports.

Recommendation: **A** (optionally + a slimmed **B** so future list-stories stage
correctly). The rest of this plan details A.

---

## Items (Option A)

> All items are in the **`shop`** repo (cross-repo commit via `--repo shop`).
> Build red→green in DSL/contract/production order so each layer has a failing
> test before the layer above it is written.

### 1. [test-kit · stub] Add an enumerable ERP product-list stub

**Where:** `ErpStubClient.java` (+ `BaseErpClient.java` if a shared path const is
reused).

**Change:** add `configureGetProducts(List<ExtProductDetailsResponse>)` that
WireMock-stubs `GET /erp/api/products` (exact path, no `/{sku}`) returning the
array. Keep the existing per-SKU `configureGetProduct` untouched.

### 2. [test-kit · driver] Expose the list on the ERP driver surface

**Where:** `ErpDriver.java` (port), `ErpStubDriver.java` / `BaseErpDriver.java`
(adapter).

**Change:** add a `returnsProducts(...)` driver step that calls the new stub.
Decide the DSL ergonomics:
- **2a (preferred):** new `given().products(...)` step that registers the list
  stub once. Leaves `given().product()` semantics (per-SKU, used by PlaceOrder
  etc.) **unchanged**.
- **2b (alt):** have repeated `given().product()` calls *also* accumulate into the
  list stub. Riskier — changes a primitive shared by other stories; only do this
  if the AT must read as singular `given().product()...product()...`.

### 3. [test-kit · contract] Pin the ERP list contract

**Where:** `BaseErpContractTest.java`.

**Change:** add `shouldBeAbleToListProducts()` staging ≥2 products via the new
step and asserting the driver returns both (sku + price). This is the contract
that authorises production to proxy the list.

### 4. [production · gateway] Add `ErpGateway.getProducts()`

**Where:** `system/monolith/java/.../core/services/external/ErpGateway.java`.

**Change:** add `List<ProductDetailsResponse> getProducts()` issuing `GET
{erpUrl}/api/products`, deserializing the array. Mirror the error handling of the
existing `getProductDetails(sku)` (non-200 → `IllegalStateException`).

### 5. [production · API] Add `GET /api/products`

**Where:** `system/monolith/java/.../api/controller/` (new `ProductApiController`)
+ whatever service/mapper layer the Order/Coupon controllers use for symmetry.

**Change:** `GET /api/products` returns the proxied ERP list mapped to the public
response shape. **No DB migration** — pass-through proxy, no `products` table.

### 6. [verify] Re-run the rehearsal for #65

**Where:** operator-driven (not agent `## Items` work) — see Verification.

---

## Verification

(Operator-driven.)

- `bash scripts/atdd-rehearsal.sh 65 --config gh-optivem-monolith-java.yaml`
  (drop `--headless` to inspect) now walks past `IMPLEMENT_AND_VERIFY_SYSTEM_API`
  without a `STOP_SCOPE_VIOLATION`; `shouldListAllProductsWhenProductsAreAvailable`
  and `shouldReturnEmptyListWhenNoProductsAreAvailable` go red→green.
- The per-SKU stories (PlaceOrder / ViewOrder / contract `shouldBeAbleToGetProduct`)
  still pass — confirms item 2 left `given().product()` semantics intact.
- The ERP contract test exercises **both** `/{sku}` and the new list path.
- Once green, re-run `bash scripts/atdd-rehearsal-loop.sh` so #65 no longer stops
  the corpus.

---

## Notes / open questions

- **Field mapping:** the simulator returns `{id, title, price, …}`; the test-kit
  `GetProductResponse` tracks `{sku, price}` (id→sku). The list response is just an
  array of the same shape — confirm whether #65's AT asserts more than sku+price
  (e.g. name) and extend the DTO only if the story requires it.
- **Empty-list scenario** (`shouldReturnEmptyListWhenNoProductsAreAvailable`) is
  already satisfiable once the list stub exists (stub returns `[]`).
- **Other configs:** this plan targets the **Java monolith**. The same gap exists
  for the dotnet / typescript variants and the multitier configs; if #65 should
  pass on those too, replicate items 1–5 per language (separate follow-up).
