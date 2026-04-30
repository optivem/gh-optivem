namespace Shop.SystemTest;

public class RegisterCustomerTest
{
    [Fact]
    public void ShouldRegisterCustomer()
    {
    }

    [Theory]
    [InlineData("foo")]
    public void ShouldRejectInvalid(string input)
    {
    }

    [Fact(DisplayName = "kept")]
    public void ShouldKeepDisplayName()
    {
    }

    [Fact(DisplayName = "kept2")]
    public void ShouldKeepDisplayName2()
    {
    }
}
