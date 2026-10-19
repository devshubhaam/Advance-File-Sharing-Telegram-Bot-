import os
import asyncio
import base64
import html
import logging
import threading

from flask import Flask
from telegram import Update, InlineKeyboardButton, InlineKeyboardMarkup
from telegram.constants import ParseMode
from telegram.ext import (
    Application,
    CommandHandler,
    MessageHandler,
    CallbackQueryHandler,
    ContextTypes,
    filters,
)

logging.basicConfig(level=logging.INFO)
log = logging.getLogger("filestore")

BOT_TOKEN = os.environ["BOT_TOKEN"]
DB_CHANNEL = int(os.environ["DB_CHANNEL"])  # private channel id, e.g. -1001234567890
ADMINS = [int(x) for x in os.environ.get("ADMINS", "").replace(" ", "").split(",") if x]
PORT = int(os.environ.get("PORT", "10000"))

# ---------- tiny web server (Render + cron ping ke liye) ----------
web = Flask(__name__)


@web.route("/")
def home():
    return "Bot is alive", 200


@web.route("/health")
def health():
    return "ok", 200


def run_web():
    web.run(host="0.0.0.0", port=PORT)


# ---------- helpers ----------
def encode(msg_id: int) -> str:
    return base64.urlsafe_b64encode(f"get-{msg_id}".encode()).decode().rstrip("=")


def decode(payload: str) -> int:
    padded = payload + "=" * (-len(payload) % 4)
    raw = base64.urlsafe_b64decode(padded.encode()).decode()
    return int(raw.split("-")[1])


def main_buttons():
    return InlineKeyboardMarkup(
        [
            [
                InlineKeyboardButton("🕵️ HELP", callback_data="help"),
                InlineKeyboardButton("📜 ABOUT", callback_data="about"),
            ],
            [InlineKeyboardButton("‼️ SETTINGS ‼️", callback_data="settings")],
        ]
    )


def back_button():
    return InlineKeyboardMarkup([[InlineKeyboardButton("⬅️ BACK", callback_data="home")]])


def welcome_text(user):
    name = html.escape(user.first_name or "User")
    bot_name = html.escape(BOT_NAME)
    return (
        f"<b><i>HEY {name},</i></b>\n\n"
        f"<blockquote><b><i>MY NAME IS {bot_name}</i></b></blockquote>\n\n"
        "<b><i>I AM A PERMENANT FILE STORE BOT AND USERS CAN ACCESS STORED "
        "MESSAGES BY USING A SHAREABLE LINK GIVEN BY ME.</i></b>\n\n"
        "<blockquote><b><i>TO KNOW MORE CLICK HELP BUTTON.</i></b></blockquote>"
    )


BOT_NAME = "FileStoreBot"  # startup pe real naam se replace hoga


# ---------- handlers ----------
async def start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    msg = update.effective_message
    user = update.effective_user

    # Link se aaya hai -> file bhejo
    if context.args:
        try:
            msg_id = decode(context.args[0])
        except Exception:
            await msg.reply_text("❌ Invalid ya expired link.")
            return
        try:
            await context.bot.copy_message(
                chat_id=msg.chat_id,
                from_chat_id=DB_CHANNEL,
                message_id=msg_id,
            )
        except Exception as e:
            log.warning("copy failed: %s", e)
            await msg.reply_text("❌ File nahi mili (delete ho gayi ho sakti hai).")
        return

    await msg.reply_text(
        welcome_text(user),
        parse_mode=ParseMode.HTML,
        reply_markup=main_buttons(),
        quote=True,
    )


async def on_button(update: Update, context: ContextTypes.DEFAULT_TYPE):
    q = update.callback_query
    await q.answer()
    d = q.data

    if d == "home":
        text, kb = welcome_text(q.from_user), main_buttons()
    elif d == "help":
        text = (
            "<b>📖 HELP</b>\n\n"
            "• Admin koi bhi file/video/photo bheje → bot link dega\n"
            "• Koi bhi us link ko open kare → file mil jayegi\n"
            "• /start - bot start karo\n"
            "• /link - reply karke link banao (admin)"
        )
        kb = back_button()
    elif d == "about":
        text = (
            "<b>📜 ABOUT</b>\n\n"
            f"• Bot: {html.escape(BOT_NAME)}\n"
            "• Language: Python\n"
            "• Library: python-telegram-bot\n"
            "• Host: Render"
        )
        kb = back_button()
    elif d == "settings":
        if q.from_user.id in ADMINS:
            text = "<b>⚙️ SETTINGS</b>\n\nAdmin panel: abhi koi setting nahi hai."
        else:
            text = "<b>⚙️ SETTINGS</b>\n\n⛔ Ye sirf admin ke liye hai."
        kb = back_button()
    else:
        return

    await q.edit_message_text(text, parse_mode=ParseMode.HTML, reply_markup=kb)


async def on_upload(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """Admin file bheje -> DB channel me copy -> link do."""
    msg = update.effective_message
    try:
        saved = await context.bot.copy_message(
            chat_id=DB_CHANNEL,
            from_chat_id=msg.chat_id,
            message_id=msg.message_id,
        )
    except Exception as e:
        await msg.reply_text(f"❌ DB channel me save nahi hua: {e}")
        return

    link = f"https://t.me/{context.bot.username}?start={encode(saved.message_id)}"
    await msg.reply_text(
        f"✅ <b>Saved!</b>\n\n🔗 <code>{link}</code>",
        parse_mode=ParseMode.HTML,
        disable_web_page_preview=True,
        quote=True,
    )


async def post_init(app: Application):
    global BOT_NAME
    me = await app.bot.get_me()
    BOT_NAME = me.first_name
    log.info("Started as @%s", me.username)


def main():
    threading.Thread(target=run_web, daemon=True).start()

    asyncio.set_event_loop(asyncio.new_event_loop())

    app = Application.builder().token(BOT_TOKEN).post_init(post_init).build()
    app.add_handler(CommandHandler("start", start))
    app.add_handler(CallbackQueryHandler(on_button))
    app.add_handler(
        MessageHandler(
            filters.User(ADMINS)
            & filters.ChatType.PRIVATE
            & ~filters.COMMAND
            & ~filters.StatusUpdate.ALL
            & (
                filters.Document.ALL
                | filters.VIDEO
                | filters.AUDIO
                | filters.PHOTO
                | filters.VOICE
                | filters.ANIMATION
                | filters.Sticker.ALL
                | filters.TEXT
            ),
            on_upload,
        )
    )
    app.run_polling(allowed_updates=Update.ALL_TYPES, drop_pending_updates=True)


if __name__ == "__main__":
    main()
