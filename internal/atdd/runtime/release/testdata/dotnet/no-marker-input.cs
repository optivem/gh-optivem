namespace Shop.SystemTest;

public class HappyPathTest
{
    [Fact]
    public void ShouldDoNothingDisabled()
    {
    }

    [Theory]
    [InlineData("foo")]
    public void ShouldStayEnabled(string input)
    {
    }
}
