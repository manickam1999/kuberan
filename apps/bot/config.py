import os
from dotenv import load_dotenv

load_dotenv()

class Config:
    TELEGRAM_BOT_TOKEN = os.getenv("TELEGRAM_BOT_TOKEN")
    API_BASE_URL = os.getenv("API_BASE_URL", "http://api:8080")
    BOT_INTERNAL_SECRET = os.getenv("BOT_INTERNAL_SECRET")
    LOG_LEVEL = os.getenv("LOG_LEVEL", "INFO")

config = Config()
