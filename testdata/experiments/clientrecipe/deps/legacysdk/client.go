package legacysdk

type Client struct{}

func New() *Client { return &Client{} }

func (*Client) Lookup(string) string { return "legacy" }
