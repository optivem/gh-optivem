namespace Shop.SystemTest;

public class RegisterCustomerTest
{
    [Fact(Skip = "AT - RED - DSL")]
    public void ShouldRegisterCustomer()
    {
    }

    [Theory(Skip = "AT - RED - SYSTEM DRIVER")]
    [InlineData("foo")]
    public void ShouldRejectInvalid(string input)
    {
    }

    [Fact(DisplayName = "kept", Skip = "AT - RED - DSL")]
    public void ShouldKeepDisplayName()
    {
    }

    [Fact(Skip = "AT - RED - DSL", DisplayName = "kept2")]
    public void ShouldKeepDisplayName2()
    {
    }
}
