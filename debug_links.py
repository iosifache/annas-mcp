#!/usr/bin/env python3
import requests
from bs4 import BeautifulSoup
import socket

# Force IPv4
original_create_connection = socket.create_connection
def create_connection_ipv4_only(address, *args, **kwargs):
    return original_create_connection(address, *args, **{**kwargs, 'family': socket.AF_INET})
socket.create_connection = create_connection_ipv4_only

url = "https://annas-archive.org/search?q=python%20programming"
session = requests.Session()
session.headers.update({'User-Agent': 'annas-mcp/2.0'})

response = session.get(url, timeout=30)
soup = BeautifulSoup(response.content, 'html.parser')

md5_links = soup.find_all('a', href=lambda x: x and x.startswith('/md5/'))
print(f"Found {len(md5_links)} MD5 links")

for i, link in enumerate(md5_links[:5]):
    text = link.get_text(strip=True)
    href = link.get('href')
    print(f"\n{i+1}. HREF: {href}")
    print(f"    TEXT: '{text}'")
    print(f"    PARENT TAG: {link.parent.name}")
    print(f"    PARENT TEXT: '{link.parent.get_text(strip=True)[:100]}'")
    
    # Look for h3 nearby
    h3_nearby = link.find_parent().find('h3') if link.find_parent() else None
    if h3_nearby:
        print(f"    H3 NEARBY: '{h3_nearby.get_text(strip=True)}'")
    
    # Print the full parent structure for the first link
    if i == 0:
        print(f"    FULL PARENT HTML: {str(link.parent)[:300]}")