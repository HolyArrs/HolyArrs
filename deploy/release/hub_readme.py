"""Make the Docker Hub copy of README.md.

Docker Hub drops <picture>, ignores image heights and stacks images, so the
GitHub README shows the light pictures on a dark page and a column of huge
buttons. This prints a copy with one picture per spot (the dark one), the
button row and badges as plain links, and repo links made absolute.

Usage: hub_readme.py [README.md] > hub.md
"""
import re
import sys

REPO = "https://github.com/HolyArrs/HolyArrs/blob/main/"


def one_picture(m: re.Match) -> str:
    block = m.group(0)
    img = re.search(r"<img\b[^>]*>", block)
    if not img:
        return block
    urls = re.findall(r'(?:srcset|src)="([^"]+)"', block)
    dark = next((u for u in urls if "-dark." in u), None)
    tag = img.group(0)
    return re.sub(r'src="[^"]+"', f'src="{dark}"', tag, count=1) if dark else tag


def button_row(m: re.Match) -> str:
    links = re.findall(r'<a href="([^"]+)">.*?alt="([^"]*)".*?</a>', m.group(0), re.S)
    if not links:
        return m.group(0)
    return '<p align="center">' + " · ".join(f'<a href="{h}"><b>{t}</b></a>' for h, t in links) + "</p>"


def convert(text: str) -> str:
    # Button and badge rows: a centred paragraph holding only linked images.
    def row(m: re.Match) -> str:
        body = m.group(0)
        if "img.shields.io" in body:
            return ""
        if "/btn-" in body:
            return button_row(m)
        return body

    text = re.sub(r'<p align="center">(?:(?!</p>).)*</p>', row, text, flags=re.S)
    text = re.sub(r"<picture>.*?</picture>", one_picture, text, flags=re.S)
    # Docker Hub keeps a centred paragraph centred, but not a centred heading.
    text = re.sub(r'<h[1-6] align="center">(.*?)</h[1-6]>', r'<p align="center"><b>\1</b></p>', text, flags=re.S)
    text = re.sub(r'href="(?!https?:|#|mailto:)([^"]+)"', lambda m: f'href="{REPO}{m.group(1)}"', text)
    text = re.sub(r"\]\((?!https?:|#|mailto:)([^)\s]+)\)", lambda m: f"]({REPO}{m.group(1)})", text)
    return re.sub(r"\n{3,}", "\n\n", text)


if __name__ == "__main__":
    path = sys.argv[1] if len(sys.argv) > 1 else "README.md"
    with open(path, encoding="utf-8") as f:
        sys.stdout.reconfigure(encoding="utf-8", newline="\n")
        sys.stdout.write(convert(f.read()))
