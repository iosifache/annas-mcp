#!/usr/bin/env python3
"""
Improved version of annas-mcp with IPv4 fix and proper search result parsing.
This version includes both IPv6 fixes and enhanced search functionality.
"""

import os
import sys
import json
import socket
import argparse
import requests
from urllib.parse import quote, urljoin
from pathlib import Path
import re
from bs4 import BeautifulSoup

# Force IPv4 for requests
import urllib3
urllib3.util.connection.HAS_IPV6 = False

class Book:
    def __init__(self, title="", hash="", language="", format="", size="", 
                 publisher="", authors="", url=""):
        self.title = title
        self.hash = hash
        self.language = language
        self.format = format
        self.size = size
        self.publisher = publisher
        self.authors = authors
        self.url = url
    
    def __str__(self):
        return f"{self.title} ({self.format}, {self.size}) - {self.hash[:8]}..."
    
    def to_dict(self):
        return {
            'title': self.title,
            'hash': self.hash,
            'language': self.language,
            'format': self.format,
            'size': self.size,
            'publisher': self.publisher,
            'authors': self.authors,
            'url': self.url
        }

class AnnasArchive:
    def __init__(self):
        self.secret_key = os.environ.get('ANNAS_SECRET_KEY', '75qvjCeMrSR6LgmR167oG2Wk4uDe5')
        self.download_path = os.environ.get('ANNAS_DOWNLOAD_PATH', '/Volumes/SSD-MacMini/Verteidigung/Download')
        
        # Create session with IPv4 preference
        self.session = requests.Session()
        self.session.headers.update({'User-Agent': 'annas-mcp/2.0'})
        
        # Force IPv4
        original_create_connection = socket.create_connection
        def create_connection_ipv4_only(address, *args, **kwargs):
            return original_create_connection(address, *args, **{**kwargs, 'family': socket.AF_INET})
        socket.create_connection = create_connection_ipv4_only
        
    def extract_meta_information(self, meta_text):
        """Extract language, format, and size from meta text"""
        parts = [p.strip() for p in meta_text.split(',')]
        language = parts[0] if len(parts) > 0 else ""
        format_info = parts[1] if len(parts) > 1 else ""
        size = parts[3] if len(parts) > 3 else ""
        return language, format_info, size
        
    def search(self, query):
        """Search for books on Anna's Archive with proper result parsing"""
        url = f"https://annas-archive.org/search?q={quote(query)}"
        
        try:
            print(f"Searching for: {query}")
            response = self.session.get(url, timeout=30)
            response.raise_for_status()
            
            soup = BeautifulSoup(response.content, 'html.parser')
            books = []
            
            # Find all MD5 links with text (skip empty image links)
            md5_links = soup.find_all('a', href=lambda x: x and x.startswith('/md5/'))
            text_links = [link for link in md5_links if link.get_text(strip=True)]
            
            # Group links by unique MD5 hash to avoid duplicates
            seen_hashes = set()
            
            for link in text_links:
                try:
                    href = link.get('href', '')
                    if not href.startswith('/md5/'):
                        continue
                    
                    # Extract MD5 hash
                    hash_match = re.match(r'/md5/([a-f0-9]+)', href)
                    if not hash_match:
                        continue
                    
                    md5_hash = hash_match.group(1)
                    
                    # Skip duplicates
                    if md5_hash in seen_hashes:
                        continue
                    seen_hashes.add(md5_hash)
                    
                    # Get title from link text
                    title = link.get_text(strip=True)
                    
                    # Try to find metadata by looking for spans with size/format info
                    meta_text = ""
                    language = ""
                    format_info = ""
                    size = ""
                    
                    # Look for metadata in the same parent container
                    parent_container = link.parent
                    while parent_container and not meta_text:
                        # Look for spans with file info
                        spans = parent_container.find_all('span')
                        for span in spans:
                            text = span.get_text(strip=True)
                            # Look for patterns like "PDF, 1.2MB" or "English, PDF"
                            if any(keyword in text.upper() for keyword in ['PDF', 'EPUB', 'MOBI', 'MB', 'KB', 'GB']):
                                meta_text = text
                                break
                        
                        if not meta_text and parent_container.parent:
                            parent_container = parent_container.parent
                        else:
                            break
                    
                    if meta_text:
                        language, format_info, size = self.extract_meta_information(meta_text)
                    
                    # Look for more metadata in divs
                    if not format_info or not size:
                        parent = link.parent
                        for _ in range(3):
                            if parent:
                                divs = parent.find_all('div', limit=10)
                                for div in divs:
                                    div_text = div.get_text(strip=True)
                                    if re.search(r'\d+(\.\d+)?\s*(MB|KB|GB)', div_text, re.IGNORECASE):
                                        # Found size info
                                        size_match = re.search(r'\d+(\.\d+)?\s*(MB|KB|GB)', div_text, re.IGNORECASE)
                                        if size_match:
                                            size = size_match.group(0)
                                    if any(fmt in div_text.upper() for fmt in ['PDF', 'EPUB', 'MOBI', 'TXT']):
                                        format_match = re.search(r'\b(PDF|EPUB|MOBI|TXT)\b', div_text, re.IGNORECASE)
                                        if format_match:
                                            format_info = format_match.group(0).upper()
                                parent = parent.parent
                    
                    book = Book(
                        title=title,
                        hash=md5_hash,
                        language=language,
                        format=format_info,
                        size=size,
                        publisher="",
                        authors="",
                        url=urljoin("https://annas-archive.org", href)
                    )
                    books.append(book)
                    
                except Exception as e:
                    print(f"Error parsing book entry: {e}")
                    continue
            
            if books:
                print(f"\nFound {len(books)} books:")
                for i, book in enumerate(books[:10], 1):  # Show first 10 results
                    print(f"{i:2d}. {book}")
                
                if len(books) > 10:
                    print(f"    ... and {len(books) - 10} more results")
                
                # Save results to JSON for further processing
                results_file = Path(self.download_path) / "search_results.json"
                with open(results_file, 'w') as f:
                    json.dump([book.to_dict() for book in books], f, indent=2)
                print(f"\nDetailed results saved to: {results_file}")
                
            else:
                print("No books found.")
                print(f"Search URL: {url}")
            
            return books
            
        except requests.RequestException as e:
            print(f"Search error: {e}")
            return []
        except Exception as e:
            print(f"Unexpected error during search: {e}")
            return []
    
    def download(self, md5_hash, filename):
        """Download a book by MD5 hash"""
        if not filename.endswith(('.pdf', '.epub', '.mobi', '.djvu', '.txt', '.doc', '.docx')):
            print("Error: Filename must include an extension (e.g., .pdf, .epub)")
            return False
            
        # Get download URL from API
        api_url = f"https://annas-archive.org/dyn/api/fast_download.json?md5={md5_hash}&key={self.secret_key}"
        
        try:
            print(f"Fetching download URL...")
            response = self.session.get(api_url, timeout=30)
            data = response.json()
            
            if 'download_url' not in data or data['download_url'] is None:
                print(f"Error: {data.get('error', 'Failed to get download URL')}")
                return False
            
            download_url = data['download_url']
            downloads_left = data.get('account_fast_download_info', {}).get('downloads_left', 'unknown')
            print(f"Downloads remaining: {downloads_left}")
            
            # Download the file
            output_path = Path(self.download_path) / filename
            print(f"Downloading to: {output_path}")
            
            response = self.session.get(download_url, stream=True, timeout=60)
            response.raise_for_status()
            
            total_size = int(response.headers.get('content-length', 0))
            downloaded = 0
            
            with open(output_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=8192):
                    if chunk:
                        f.write(chunk)
                        downloaded += len(chunk)
                        if total_size > 0:
                            percent = (downloaded / total_size) * 100
                            print(f"Progress: {percent:.1f}%", end='\r')
            
            print(f"\nSuccessfully downloaded: {output_path}")
            return True
            
        except requests.RequestException as e:
            print(f"Download error: {e}")
            return False
        except json.JSONDecodeError:
            print("Error: Invalid response from API")
            return False
        except Exception as e:
            print(f"Unexpected error: {e}")
            return False
    
    def mcp_server(self):
        """Start MCP server mode"""
        print("MCP server mode not implemented in this version")
        print("Use the original binary for MCP server functionality")
        return False

def main():
    parser = argparse.ArgumentParser(
        description='Improved command-line interface for Anna\'s Archive with IPv6 fixes and proper search parsing.'
    )
    
    subparsers = parser.add_subparsers(dest='command', help='Available commands')
    
    # Search command
    search_parser = subparsers.add_parser('search', help='Search for books with detailed results')
    search_parser.add_argument('query', help='Search query')
    
    # Download command
    download_parser = subparsers.add_parser('download', help='Download a book by its MD5 hash')
    download_parser.add_argument('hash', help='MD5 hash of the book')
    download_parser.add_argument('filename', help='Output filename (must include extension)')
    
    # MCP command
    mcp_parser = subparsers.add_parser('mcp', help='Start the MCP server')
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        return
    
    anna = AnnasArchive()
    
    if args.command == 'search':
        anna.search(args.query)
    elif args.command == 'download':
        anna.download(args.hash, args.filename)
    elif args.command == 'mcp':
        anna.mcp_server()
    else:
        parser.print_help()

if __name__ == '__main__':
    main()