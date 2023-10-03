package commands

type Command struct {
	Name     string
	ChatId   int64
	ThreadId int
	Text     string
}

type Content struct {
	Text     string
	ThreadId int
}
