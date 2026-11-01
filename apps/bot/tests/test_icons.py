import os
import sys

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from utils.formatting import display_icon, is_emoji_icon  # noqa: E402


def test_emoji_icons_are_accepted():
    for icon in ["🍕", "☕️", "👍🏽", "👩‍💻", "🇲🇾"]:
        assert is_emoji_icon(icon), icon


def test_text_and_named_icons_are_rejected():
    for icon in ["", "food", "fo", "🍕 food", "1", "shopping-cart", "食"]:
        assert not is_emoji_icon(icon), icon


def test_display_icon_hides_named_icons():
    assert display_icon("🛒") == "🛒"
    assert display_icon("shopping-cart") == ""
    assert display_icon("landmark") == ""
    assert display_icon("") == ""
