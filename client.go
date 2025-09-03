package gosocket

type Client struct {
	ID          string
	Conn        interface{} // WebSocket connection (gorilla/websocket.Conn)
	MessageChan chan []byte
	Hub         *Hub
	UserData    map[string]interface{} // user custom data
}

func NewClient(id string, conn interface{}, hub *Hub) *Client {
	return &Client{
		ID:          id,
		Conn:        conn,
		Hub:         hub,
		MessageChan: make(chan []byte, 256),
		UserData:    make(map[string]interface{}),
	}
}

func (c *Client) Send(message []byte) error {
	return nil
}

func (c *Client) SendMessage(message *Message) error {
	return nil
}

func (c *Client) SendData(data interface{}) error {
	return nil
}

func (c *Client) SendDataWithEncoding(data interface{}, encoding EncodingType) error {
	return nil
}

func (c *Client) SendJSON(data interface{}) error {
	return nil
}

func (c *Client) SendProtobuf(data interface{}) error {
	return nil
}

func (c *Client) JoinRoom(room string) error {
	return nil
}

func (c *Client) LeaveRoom(room string) error {
	return nil
}

func (c *Client) GetRooms() []string {
	return nil
}

func (c *Client) Disconnect() error {
	return nil
}

func (c *Client) SetUserData(key string, value interface{}) {
}

func (c *Client) GetUserData(key string) interface{} {
	return nil
}
