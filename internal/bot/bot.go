package bot

import (
	shortenerv1 "GURLS-Bot/gen/go/shortener/v1"
	"GURLS-Bot/internal/config"
	"GURLS-Bot/internal/grpc/client"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Bot message constants
const (
	msgHelp = `URL Shortener Bot

Create and manage short links efficiently.
Select an action below:`
	msgUseShortenCommand         = "Send a URL to create a short link or use the buttons below:"
	msgInvalidShortenFormat      = "Invalid format. Please send a valid URL (e.g., https://example.com)"
	msgLinkSuccessfullyShortened = "Link created successfully.\n\nShort URL: %s"
	msgLinkStats                 = "Link Statistics: %s%s\n\nOriginal URL: %s\nTotal Clicks: %d\nExpires: %s%s"
	msgUnknownCommand            = "Unknown command. Use /start to see available options."
	msgInvalidCommandFormat      = "Invalid command format. Use: /%s <alias>"
	msgLinkNotFound              = "Link with alias '%s' not found."
	msgInternalError             = "Internal error occurred. Please try again later."
	msgLinkDeleted               = "Link '%s' has been deleted successfully."
	msgMyLinksHeader             = "Your Links:"
	msgNoLinks                   = "You have no links yet.\nCreate your first link!"
	msgAliasTaken                = "Alias '%s' is already taken. Please choose another one."

	// Callback data constants
	callbackCreateLink   = "create_link"
	callbackMyLinks      = "my_links"  
	callbackHelp         = "help"
	callbackCancel       = "cancel"
	callbackCustomAlias  = "custom_alias"

	// Additional messages
	msgSendCustomAlias   = "Send your custom alias (letters, numbers, hyphens only):"
	msgSendUrlWithAlias  = "Now send the URL you want to shorten with alias '%s':"
)

var (
	urlRegex       = regexp.MustCompile(`https?://\S+`)
	titleRegex     = regexp.MustCompile(`title="([^"]+)"`)
	expiresInRegex = regexp.MustCompile(`expires_in=([\w\d]+)`)
	aliasRegex     = regexp.MustCompile(`alias=([\w\-]+)`)
	customAliasRegex = regexp.MustCompile(`^[a-zA-Z0-9\-]{1,20}$`)
)

// User state management
type UserState struct {
	State       string
	CustomAlias string
}

const (
	StateNormal           = "normal"
	StateWaitingForAlias  = "waiting_for_alias"
	StateWaitingForURL    = "waiting_for_url"
)

type Bot struct {
	api        *tgbotapi.BotAPI
	log        *zap.Logger
	config     *config.Config
	grpcClient *client.BackendClient
	userStates map[int64]*UserState
}

func New(cfg *config.Config, log *zap.Logger, grpcClient *client.BackendClient) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return nil, err
	}
	log.Info("authorized on account", zap.String("username", api.Self.UserName))
	return &Bot{
		api:        api, 
		log:        log, 
		config:     cfg, 
		grpcClient: grpcClient,
		userStates: make(map[int64]*UserState),
	}, nil
}

func (b *Bot) Start(ctx context.Context) {
	b.log.Info("starting bot")
	updates := b.getUpdatesChannel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				b.log.Info("stopping bot...")
				b.api.StopReceivingUpdates()
				return
			case update := <-updates:
				b.processUpdate(update)
			}
		}
	}()
}

func (b *Bot) processUpdate(update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		if err := b.handleCallbackQuery(update.CallbackQuery); err != nil {
			b.log.Error("failed to handle callback query", zap.Error(err))
		}
		return
	}
	
	if update.Message == nil {
		return
	}
	
	if update.Message.IsCommand() {
		if err := b.handleCommand(update.Message); err != nil {
			b.log.Error("failed to handle command", zap.String("command", update.Message.Command()), zap.Error(err))
		}
		return
	}
	
	if err := b.handleMessage(update.Message); err != nil {
		b.log.Error("failed to handle message", zap.Error(err))
	}
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) error {
	switch msg.Command() {
	case "start":
		return b.sendMessageWithKeyboard(msg.Chat.ID, msgHelp, b.createMainKeyboard())
	case "shorten":
		return b.handleShortenCommand(msg.Chat.ID, msg.CommandArguments())
	case "stats":
		return b.handleStatsCommand(msg.Chat.ID, msg.CommandArguments())
	case "delete":
		return b.handleDeleteCommand(msg.Chat.ID, msg.CommandArguments())
	case "my_links":
		return b.handleMyLinksCommand(msg.Chat.ID)
	default:
		return b.sendMessage(msg.Chat.ID, msgUnknownCommand, false)
	}
}

// Handle shorten command with URL parsing
func (b *Bot) handleShortenCommand(chatID int64, args string) error {
	urlMatch := urlRegex.FindString(args)
	if urlMatch == "" {
		return b.sendMessage(chatID, msgInvalidShortenFormat, true)
	}

	req := &shortenerv1.CreateLinkRequest{OriginalUrl: urlMatch, UserTgId: chatID}

	if titleMatch := titleRegex.FindStringSubmatch(args); len(titleMatch) > 1 {
		title := titleMatch[1]
		req.Title = &title
	}
	if aliasMatch := aliasRegex.FindStringSubmatch(args); len(aliasMatch) > 1 {
		alias := aliasMatch[1]
		req.CustomAlias = &alias
	}
	if expiresInMatch := expiresInRegex.FindStringSubmatch(args); len(expiresInMatch) > 1 {
		duration, err := time.ParseDuration(expiresInMatch[1])
		if err == nil {
			req.ExpiresAt = timestamppb.New(time.Now().Add(duration))
		}
	}

	res, err := b.grpcClient.CreateLink(context.Background(), req)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
			return b.sendMessage(chatID, fmt.Sprintf(msgAliasTaken, *req.CustomAlias), false)
		}
		b.log.Error("gRPC CreateLink failed", zap.Error(err))
		return b.sendMessage(chatID, msgInternalError, false)
	}
	shortURL := fmt.Sprintf("%s/%s", b.config.HTTPServer.BaseURL, res.GetAlias())
	message := fmt.Sprintf(msgLinkSuccessfullyShortened, shortURL)
	return b.sendMessageWithKeyboard(chatID, message, b.createLinkActionsKeyboard(res.GetAlias()))
}

func (b *Bot) handleMyLinksCommand(chatID int64) error {
	req := &shortenerv1.ListUserLinksRequest{UserTgId: chatID}
	res, err := b.grpcClient.ListUserLinks(context.Background(), req)
	if err != nil {
		b.log.Error("gRPC ListUserLinks failed", zap.Error(err))
		return b.sendMessage(chatID, msgInternalError, false)
	}
	if len(res.Links) == 0 {
		return b.sendMessageWithKeyboard(chatID, msgNoLinks, b.createMainKeyboard())
	}

	var builder strings.Builder
	builder.WriteString(msgMyLinksHeader)
	
	var keyboardRows [][]tgbotapi.InlineKeyboardButton
	
	for i, link := range res.Links {
		title := link.GetOriginalUrl()
		if link.Title != nil && *link.Title != "" {
			title = *link.Title
		}
		
		// Limit title length for clean display
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		
		builder.WriteString(fmt.Sprintf("\n\n%d. %s\n   %s/%s", i+1, title, b.config.HTTPServer.BaseURL, link.Alias))
		
		// Add action buttons for each link
		keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Stats", "stats_"+link.Alias),
			tgbotapi.NewInlineKeyboardButtonData("Delete", "delete_"+link.Alias),
		))
	}
	
	// Add navigation buttons
	keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Create Link", callbackCreateLink),
	))
	keyboardRows = append(keyboardRows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Main Menu", callbackHelp),
	))
	
	keyboard := tgbotapi.InlineKeyboardMarkup{InlineKeyboard: keyboardRows}
	return b.sendMessageWithKeyboard(chatID, builder.String(), keyboard)
}

func (b *Bot) handleStatsCommand(chatID int64, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return b.sendMessage(chatID, fmt.Sprintf(msgInvalidCommandFormat, "stats"), false)
	}

	req := &shortenerv1.GetLinkStatsRequest{Alias: alias}
	res, err := b.grpcClient.GetLinkStats(context.Background(), req)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return b.sendMessage(chatID, fmt.Sprintf(msgLinkNotFound, alias), false)
		}
		b.log.Error("gRPC GetLinkStats failed", zap.Error(err), zap.String("alias", alias))
		return b.sendMessage(chatID, msgInternalError, false)
	}

	expiresText := "Never"
	if res.ExpiresAt != nil {
		expiresText = res.ExpiresAt.AsTime().Format("2006-01-02 15:04 MST")
	}

	var titleText string
	if res.Title != nil && *res.Title != "" {
		titleText = fmt.Sprintf("\nTitle: %s", *res.Title)
	}

	deviceStatsBuilder := &strings.Builder{}
	if len(res.ClicksByDevice) > 0 {
		deviceStatsBuilder.WriteString("\n\nBy Device:")
		for device, count := range res.ClicksByDevice {
			deviceStatsBuilder.WriteString(fmt.Sprintf("\n- %s: %d", device, count))
		}
	}

	responseText := fmt.Sprintf("Link Statistics: %s%s\n\nOriginal URL: %s\nTotal Clicks: %d\nExpires: %s%s",
		alias, titleText, res.OriginalUrl, res.ClickCount, expiresText, deviceStatsBuilder.String())

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Delete", "delete_"+alias),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My Links", callbackMyLinks),
			tgbotapi.NewInlineKeyboardButtonData("Menu", callbackHelp),
		),
	)
	return b.sendMessageWithKeyboard(chatID, responseText, keyboard)
}

func (b *Bot) handleDeleteCommand(chatID int64, alias string) error {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return b.sendMessage(chatID, fmt.Sprintf(msgInvalidCommandFormat, "delete"), false)
	}
	req := &shortenerv1.DeleteLinkRequest{Alias: alias}
	err := b.grpcClient.DeleteLink(context.Background(), req)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
			return b.sendMessage(chatID, fmt.Sprintf(msgLinkNotFound, alias), false)
		}
		b.log.Error("gRPC DeleteLink failed", zap.Error(err), zap.String("alias", alias))
		return b.sendMessage(chatID, msgInternalError, false)
	}
	responseText := fmt.Sprintf(msgLinkDeleted, alias)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Create Link", callbackCreateLink),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My Links", callbackMyLinks),
			tgbotapi.NewInlineKeyboardButtonData("Menu", callbackHelp),
		),
	)
	return b.sendMessageWithKeyboard(chatID, responseText, keyboard)
}

func (b *Bot) handleMessage(msg *tgbotapi.Message) error {
	userID := msg.Chat.ID
	state := b.getUserState(userID)
	
	switch state.State {
	case StateWaitingForAlias:
		return b.handleCustomAliasInput(userID, msg.Text)
	case StateWaitingForURL:
		return b.handleURLInputWithAlias(userID, msg.Text, state.CustomAlias)
	default:
		// Default behavior - check if it's a URL
		if urlRegex.MatchString(msg.Text) {
			return b.handleShortenCommand(userID, msg.Text)
		}
		return b.sendMessageWithKeyboard(userID, msgUseShortenCommand, b.createMainKeyboard())
	}
}

func (b *Bot) sendMessage(chatID int64, text string, useMarkdown bool) error {
	reply := tgbotapi.NewMessage(chatID, text)
	if useMarkdown {
		reply.ParseMode = tgbotapi.ModeMarkdown
	}
	_, err := b.api.Send(reply)
	return err
}

// Handle callback queries from inline buttons
func (b *Bot) handleCallbackQuery(callback *tgbotapi.CallbackQuery) error {
	// Answer callback to remove loading spinner
	answerCallback := tgbotapi.NewCallback(callback.ID, "")
	if _, err := b.api.Request(answerCallback); err != nil {
		b.log.Error("failed to answer callback", zap.Error(err))
	}

	switch {
	case callback.Data == callbackCreateLink:
		return b.sendMessageWithKeyboard(callback.Message.Chat.ID, "Send a URL to create a short link:", b.createCreateLinkKeyboard())
	case callback.Data == callbackMyLinks:
		return b.handleMyLinksCommand(callback.Message.Chat.ID)
	case callback.Data == callbackHelp:
		return b.sendMessageWithKeyboard(callback.Message.Chat.ID, msgHelp, b.createMainKeyboard())
	case strings.HasPrefix(callback.Data, "stats_"):
		alias := strings.TrimPrefix(callback.Data, "stats_")
		return b.handleStatsCommand(callback.Message.Chat.ID, alias)
	case strings.HasPrefix(callback.Data, "delete_"):
		alias := strings.TrimPrefix(callback.Data, "delete_")
		return b.handleDeleteCommand(callback.Message.Chat.ID, alias)
	case callback.Data == callbackCustomAlias:
		b.setUserState(callback.Message.Chat.ID, StateWaitingForAlias, "")
		return b.sendMessage(callback.Message.Chat.ID, msgSendCustomAlias, false)
	}
	
	return nil
}

// Create main menu keyboard
func (b *Bot) createMainKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Create Link", callbackCreateLink),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My Links", callbackMyLinks),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Help", callbackHelp),
		),
	)
}

// Create keyboard for successfully created link
func (b *Bot) createLinkActionsKeyboard(alias string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Statistics", "stats_"+alias),
			tgbotapi.NewInlineKeyboardButtonData("Delete", "delete_"+alias),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("My Links", callbackMyLinks),
			tgbotapi.NewInlineKeyboardButtonData("Create Another", callbackCreateLink),
		),
	)
}

// Create link creation options keyboard
func (b *Bot) createCreateLinkKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Use Custom Alias", callbackCustomAlias),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Back to Menu", callbackHelp),
		),
	)
}

// Send message with inline keyboard
func (b *Bot) sendMessageWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	_, err := b.api.Send(msg)
	return err
}

// User state management methods
func (b *Bot) getUserState(userID int64) *UserState {
	if state, exists := b.userStates[userID]; exists {
		return state
	}
	return &UserState{State: StateNormal}
}

func (b *Bot) setUserState(userID int64, state string, customAlias string) {
	b.userStates[userID] = &UserState{
		State:       state,
		CustomAlias: customAlias,
	}
}

func (b *Bot) resetUserState(userID int64) {
	delete(b.userStates, userID)
}

// Handle custom alias input
func (b *Bot) handleCustomAliasInput(userID int64, alias string) error {
	alias = strings.TrimSpace(alias)
	
	if !customAliasRegex.MatchString(alias) {
		return b.sendMessage(userID, "Invalid alias format. Use only letters, numbers, and hyphens (1-20 characters).", false)
	}
	
	b.setUserState(userID, StateWaitingForURL, alias)
	return b.sendMessage(userID, fmt.Sprintf(msgSendUrlWithAlias, alias), false)
}

// Handle URL input with custom alias
func (b *Bot) handleURLInputWithAlias(userID int64, text string, customAlias string) error {
	defer b.resetUserState(userID)
	
	urlMatch := urlRegex.FindString(text)
	if urlMatch == "" {
		return b.sendMessage(userID, msgInvalidShortenFormat, false)
	}
	
	req := &shortenerv1.CreateLinkRequest{
		OriginalUrl: urlMatch,
		UserTgId:    userID,
		CustomAlias: &customAlias,
	}
	
	res, err := b.grpcClient.CreateLink(context.Background(), req)
	if err != nil {
		if st, ok := status.FromError(err); ok && st.Code() == codes.AlreadyExists {
			return b.sendMessage(userID, fmt.Sprintf(msgAliasTaken, customAlias), false)
		}
		b.log.Error("gRPC CreateLink failed", zap.Error(err))
		return b.sendMessage(userID, msgInternalError, false)
	}
	
	shortURL := fmt.Sprintf("%s/%s", b.config.HTTPServer.BaseURL, res.GetAlias())
	message := fmt.Sprintf(msgLinkSuccessfullyShortened, shortURL)
	return b.sendMessageWithKeyboard(userID, message, b.createLinkActionsKeyboard(res.GetAlias()))
}

func (b *Bot) getUpdatesChannel() tgbotapi.UpdatesChannel {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	return b.api.GetUpdatesChan(u)
}
