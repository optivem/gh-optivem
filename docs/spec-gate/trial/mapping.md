# Place Order: criteria -> existing latest/ tests (acceptance + e2e)
| Criterion | Tests |
|---|---|
| 1 | acceptance.PlaceOrderPositiveTest: shouldBeAbleToPlaceOrderForValidInput, orderStatusShouldBePlaced..., orderPrefixShouldBeORD; e2e.PlaceOrderPositiveTest.shouldPlaceOrder |
| 2 | shouldCalculateBasePrice..., shouldPlaceOrderWithCorrectBasePriceParameterized |
| 3 | discountRateShouldBeAppliedForCoupon, ...NotAppliedWhenNoCoupon, subtotal x2, orderTotalShouldReflectCouponDiscount |
| 4 | correctTaxRate..., totalPriceShouldBeSubtotalPlusTax, orderTotalShouldIncludeTax, orderTotalShouldApplyCouponDiscountAndTax (also touches 3) |
| 5 | couponUsageCountHasBeenIncremented... |
| 6 | Isolated.shouldRecordPlacementTimestamp |
| 7 | Isolated.shouldApplyFullPriceWithoutPromotion, ...DiscountWhenPromotionIsActive |
| 8 | Invalid/Negative/Zero/Empty/NonInteger/Null quantity (6 tests) |
| 9 | NonExistentSku, EmptySku, NullSku |
| 10 | EmptyCountry, InvalidCountry, NullCountry |
| 11 | NonExistentCoupon, ExceededUsageLimit, Isolated.cannotPlaceOrderWithExpiredCoupon |
| 12 | adminShouldNotBeAbleToPlaceOrder |
| 13 | Isolated.shouldRejectOrderPlacedAtYearEnd |
| 14 | NO TEST (narrative 5a) |
Findings: criteria 3/4 share tests (many-to-many); criterion 14 uncovered; narrative has 3 extensions but tests cover ~9 more behaviours (coupons, tax, promo, admin, year-end), so the narrative is not the full spec. Tests are @TestTemplate (run per channel API/UI), so one method = many JUnit results; legacy/ has ~25 duplicate PlaceOrder tests to exclude.
