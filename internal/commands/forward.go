package commands

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/utils"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/gotd/td/tg"
)

const maxForwardIDs = 50

func (m *command) LoadForward(dispatcher dispatcher.Dispatcher) {
	log := m.log.Named("forward")
	defer log.Sugar().Info("Loaded")
	dispatcher.AddHandler(handlers.NewCommand("get", getCmd))
	dispatcher.AddHandler(handlers.NewCommand("post", postCmd))
}

// parseIDs turns "5 7 10-12" into [5 7 10 11 12].
func parseIDs(args []string) ([]int, error) {
	var ids []int
	for _, a := range args {
		if strings.Contains(a, "-") {
			parts := strings.SplitN(a, "-", 2)
			from, err1 := strconv.Atoi(parts[0])
			to, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || from <= 0 || to < from {
				return nil, fmt.Errorf("invalid range %q", a)
			}
			for i := from; i <= to; i++ {
				ids = append(ids, i)
				if len(ids) > maxForwardIDs {
					return nil, fmt.Errorf("too many messages, max %d at a time", maxForwardIDs)
				}
			}
			continue
		}
		id, err := strconv.Atoi(a)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid message id %q", a)
		}
		ids = append(ids, id)
		if len(ids) > maxForwardIDs {
			return nil, fmt.Errorf("too many messages, max %d at a time", maxForwardIDs)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no message id given")
	}
	return ids, nil
}

// isVideoOrImage reports whether a message carries a photo, a video or an image file.
func isVideoOrImage(msg *tg.Message) bool {
	switch media := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		return true
	case *tg.MessageMediaDocument:
		doc, ok := media.Document.AsNotEmpty()
		if !ok {
			return false
		}
		if strings.HasPrefix(doc.MimeType, "video/") || strings.HasPrefix(doc.MimeType, "image/") {
			return true
		}
		for _, attr := range doc.Attributes {
			if _, ok := attr.(*tg.DocumentAttributeVideo); ok {
				return true
			}
		}
	}
	return false
}

// copyFromDatabase copies the video/image messages with the given IDs from the
// database channel (LOG_CHANNEL) to toPeer. Returns how many were sent.
func copyFromDatabase(ctx *ext.Context, toPeer tg.InputPeerClass, ids []int) (int, error) {
	logChannel, err := utils.GetLogChannelPeer(ctx, ctx.Raw, ctx.PeerStorage)
	if err != nil {
		return 0, err
	}
	inputIDs := make([]tg.InputMessageClass, 0, len(ids))
	for _, id := range ids {
		inputIDs = append(inputIDs, &tg.InputMessageID{ID: id})
	}
	res, err := ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: logChannel,
		ID:      inputIDs,
	})
	if err != nil {
		return 0, err
	}
	msgs, ok := res.(*tg.MessagesChannelMessages)
	if !ok {
		return 0, fmt.Errorf("unexpected response from Telegram")
	}
	var sendIDs []int
	for _, mc := range msgs.Messages {
		if msg, ok := mc.(*tg.Message); ok && isVideoOrImage(msg) {
			sendIDs = append(sendIDs, msg.ID)
		}
	}
	if len(sendIDs) == 0 {
		return 0, nil
	}
	randomIDs := make([]int64, len(sendIDs))
	for i := range randomIDs {
		randomIDs[i] = rand.Int63()
	}
	_, err = ctx.Raw.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		DropAuthor: true, // send as a copy, without the "Forwarded from" header
		Noforwards: config.ValueOf.ProtectContent,
		FromPeer:   &tg.InputPeerChannel{ChannelID: logChannel.ChannelID, AccessHash: logChannel.AccessHash},
		ID:         sendIDs,
		RandomID:   randomIDs,
		ToPeer:     toPeer,
	})
	if err != nil {
		return 0, err
	}
	return len(sendIDs), nil
}

// /get <id> [id ...] [from-to]  -> copy to the user's own chat
func getCmd(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()
	peerChatId := ctx.PeerStorage.GetPeerById(chatId)
	if peerChatId.Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("You are not allowed to use this bot."), nil)
		return dispatcher.EndGroups
	}
	ids, err := parseIDs(strings.Fields(u.EffectiveMessage.GetMessage())[1:])
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString("Usage: /get <message id> (e.g. /get 12 15 20-25)\n"+err.Error()), nil)
		return dispatcher.EndGroups
	}
	toPeer := ctx.PeerStorage.GetInputPeerById(chatId)
	n, err := copyFromDatabase(ctx, toPeer, ids)
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	if n == 0 {
		ctx.Reply(u, ext.ReplyTextString("No video or image found for the given id(s)."), nil)
	}
	return dispatcher.EndGroups
}

// /post <id> [id ...] [from-to]  -> admin only, copy to FORWARD_CHANNEL
func postCmd(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()
	peerChatId := ctx.PeerStorage.GetPeerById(chatId)
	if peerChatId.Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}
	if !utils.Contains(config.ValueOf.AdminUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("Only admins can use /post."), nil)
		return dispatcher.EndGroups
	}
	if config.ValueOf.ForwardChannelID == 0 {
		ctx.Reply(u, ext.ReplyTextString("FORWARD_CHANNEL is not set."), nil)
		return dispatcher.EndGroups
	}
	ids, err := parseIDs(strings.Fields(u.EffectiveMessage.GetMessage())[1:])
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString("Usage: /post <message id> (e.g. /post 12 15 20-25)\n"+err.Error()), nil)
		return dispatcher.EndGroups
	}
	target, err := utils.GetChannelInput(ctx, ctx.Raw, ctx.PeerStorage, config.ValueOf.ForwardChannelID)
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	n, err := copyFromDatabase(ctx, &tg.InputPeerChannel{ChannelID: target.ChannelID, AccessHash: target.AccessHash}, ids)
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Posted %d video/image message(s) to the channel.", n)), nil)
	return dispatcher.EndGroups
}
