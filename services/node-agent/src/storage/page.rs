use std::fs::{File, OpenOptions};
use std::io::{Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};
use byteorder::{BigEndian, ReadBytesExt, WriteBytesExt};
use crc32fast::Hasher;

pub const PAGE_SIZE: usize = 4096;
pub const HEADER_SIZE: usize = 32;
pub const PAGE_PAYLOAD_SIZE: usize = PAGE_SIZE - HEADER_SIZE;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum PageType {
    Free = 0,
    Data = 1,
    Index = 2,
    Header = 3,
}

impl PageType {
    pub fn from_u8(v: u8) -> Self {
        match v {
            1 => PageType::Data,
            2 => PageType::Index,
            3 => PageType::Header,
            _ => PageType::Free,
        }
    }
}

#[derive(Debug, Clone)]
pub struct PageHeader {
    pub page_id: u64,
    pub page_type: PageType,
    pub lsn: u64,
    pub checksum: u32,
    pub num_records: u16,
}

impl PageHeader {
    pub fn serialize(&self) -> [u8; HEADER_SIZE] {
        let mut buf = [0u8; HEADER_SIZE];
        let mut cursor = &mut buf[..];
        cursor.write_u64::<BigEndian>(self.page_id).unwrap();
        cursor.write_u8(self.page_type as u8).unwrap();
        cursor.write_u64::<BigEndian>(self.lsn).unwrap();
        cursor.write_u32::<BigEndian>(self.checksum).unwrap();
        cursor.write_u16::<BigEndian>(self.num_records).unwrap();
        buf
    }

    pub fn deserialize(buf: &[u8; HEADER_SIZE]) -> Result<Self, String> {
        let mut cursor = &buf[..];
        let page_id = cursor.read_u64::<BigEndian>().map_err(|e| e.to_string())?;
        let page_type_raw = cursor.read_u8().map_err(|e| e.to_string())?;
        let lsn = cursor.read_u64::<BigEndian>().map_err(|e| e.to_string())?;
        let checksum = cursor.read_u32::<BigEndian>().map_err(|e| e.to_string())?;
        let num_records = cursor.read_u16::<BigEndian>().map_err(|e| e.to_string())?;

        Ok(PageHeader {
            page_id,
            page_type: PageType::from_u8(page_type_raw),
            lsn,
            checksum,
            num_records,
        })
    }
}

#[derive(Debug, Clone)]
pub struct Page {
    pub header: PageHeader,
    pub data: [u8; PAGE_PAYLOAD_SIZE],
}

impl Page {
    pub fn new(page_id: u64, page_type: PageType, lsn: u64) -> Self {
        Page {
            header: PageHeader {
                page_id,
                page_type,
                lsn,
                checksum: 0,
                num_records: 0,
            },
            data: [0u8; PAGE_PAYLOAD_SIZE],
        }
    }

    pub fn calculate_checksum(&self) -> u32 {
        let mut hasher = Hasher::new();
        let mut header_bytes = [0u8; HEADER_SIZE];
        let mut cursor = &mut header_bytes[..];
        cursor.write_u64::<BigEndian>(self.header.page_id).unwrap();
        cursor.write_u8(self.header.page_type as u8).unwrap();
        cursor.write_u64::<BigEndian>(self.header.lsn).unwrap();
        cursor.write_u32::<BigEndian>(0).unwrap(); // checksum field zeroed during computation
        cursor.write_u16::<BigEndian>(self.header.num_records).unwrap();

        hasher.update(&header_bytes);
        hasher.update(&self.data);
        hasher.finalize()
    }

    pub fn update_checksum(&mut self) {
        self.header.checksum = self.calculate_checksum();
    }

    pub fn verify_checksum(&self) -> bool {
        self.header.checksum == self.calculate_checksum()
    }

    pub fn serialize(&mut self) -> [u8; PAGE_SIZE] {
        self.update_checksum();
        let mut buf = [0u8; PAGE_SIZE];
        let header_buf = self.header.serialize();
        buf[..HEADER_SIZE].copy_from_slice(&header_buf);
        buf[HEADER_SIZE..].copy_from_slice(&self.data);
        buf
    }

    pub fn deserialize(buf: &[u8; PAGE_SIZE]) -> Result<Self, String> {
        let mut header_buf = [0u8; HEADER_SIZE];
        header_buf.copy_from_slice(&buf[..HEADER_SIZE]);
        let header = PageHeader::deserialize(&header_buf)?;

        let mut data = [0u8; PAGE_PAYLOAD_SIZE];
        data.copy_from_slice(&buf[HEADER_SIZE..]);

        let page = Page { header, data };
        if !page.verify_checksum() {
            return Err(format!(
                "Page checksum mismatch for page_id {}: expected {}, computed {}",
                page.header.page_id,
                page.header.checksum,
                page.calculate_checksum()
            ));
        }

        Ok(page)
    }
}

pub struct PageManager {
    file_path: PathBuf,
    file: File,
    free_pages: Vec<u64>,
    next_page_id: u64,
}

impl PageManager {
    pub fn open<P: AsRef<Path>>(path: P) -> Result<Self, String> {
        let path_buf = path.as_ref().to_path_buf();
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .open(&path_buf)
            .map_err(|e| format!("Failed to open page file: {}", e))?;

        let metadata = file.metadata().map_err(|e| e.to_string())?;
        let file_len = metadata.len();
        let total_pages = (file_len / PAGE_SIZE as u64) as u64;

        let mut pm = PageManager {
            file_path: path_buf,
            file,
            free_pages: Vec::new(),
            next_page_id: total_pages,
        };

        // Scan pages to find free pages
        for pid in 0..total_pages {
            if let Ok(page) = pm.read_page(pid) {
                if page.header.page_type == PageType::Free {
                    pm.free_pages.push(pid);
                }
            }
        }

        Ok(pm)
    }

    pub fn allocate_page(&mut self, page_type: PageType, lsn: u64) -> Result<Page, String> {
        let page_id = if let Some(pid) = self.free_pages.pop() {
            pid
        } else {
            let pid = self.next_page_id;
            self.next_page_id += 1;
            pid
        };

        let mut page = Page::new(page_id, page_type, lsn);
        self.write_page(&mut page)?;
        Ok(page)
    }

    pub fn free_page(&mut self, page_id: u64) -> Result<(), String> {
        let mut page = Page::new(page_id, PageType::Free, 0);
        self.write_page(&mut page)?;
        if !self.free_pages.contains(&page_id) {
            self.free_pages.push(page_id);
        }
        Ok(())
    }

    pub fn read_page(&mut self, page_id: u64) -> Result<Page, String> {
        let offset = page_id * PAGE_SIZE as u64;
        self.file
            .seek(SeekFrom::Start(offset))
            .map_err(|e| format!("Seek failed for page_id {}: {}", page_id, e))?;

        let mut buf = [0u8; PAGE_SIZE];
        self.file
            .read_exact(&mut buf)
            .map_err(|e| format!("Read failed for page_id {}: {}", page_id, e))?;

        Page::deserialize(&buf)
    }

    pub fn write_page(&mut self, page: &mut Page) -> Result<(), String> {
        let offset = page.header.page_id * PAGE_SIZE as u64;
        let buf = page.serialize();

        self.file
            .seek(SeekFrom::Start(offset))
            .map_err(|e| format!("Seek failed for page_id {}: {}", page.header.page_id, e))?;

        self.file
            .write_all(&buf)
            .map_err(|e| format!("Write failed for page_id {}: {}", page.header.page_id, e))?;

        self.file.flush().map_err(|e| e.to_string())?;

        Ok(())
    }

    pub fn get_next_page_id(&self) -> u64 {
        self.next_page_id
    }

    pub fn get_free_page_count(&self) -> usize {
        self.free_pages.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn test_page_header_serialization() {
        let header = PageHeader {
            page_id: 42,
            page_type: PageType::Data,
            lsn: 1001,
            checksum: 0xDEADBEEF,
            num_records: 15,
        };

        let bytes = header.serialize();
        let deserialized = PageHeader::deserialize(&bytes).unwrap();

        assert_eq!(deserialized.page_id, 42);
        assert_eq!(deserialized.page_type, PageType::Data);
        assert_eq!(deserialized.lsn, 1001);
        assert_eq!(deserialized.checksum, 0xDEADBEEF);
        assert_eq!(deserialized.num_records, 15);
    }

    #[test]
    fn test_page_write_read_byte_correctness() {
        let temp_file = NamedTempFile::new().unwrap();
        let path = temp_file.path().to_path_buf();

        let mut pm = PageManager::open(&path).unwrap();
        let mut page = pm.allocate_page(PageType::Data, 100).unwrap();

        let payload_text = b"NimbusDB fixed 4KB page manager test data payload.";
        page.data[..payload_text.len()].copy_from_slice(payload_text);
        page.header.num_records = 1;

        pm.write_page(&mut page).unwrap();

        let read_page = pm.read_page(page.header.page_id).unwrap();
        assert_eq!(read_page.header.page_id, page.header.page_id);
        assert_eq!(read_page.header.page_type, PageType::Data);
        assert_eq!(read_page.header.lsn, 100);
        assert_eq!(&read_page.data[..payload_text.len()], payload_text);
        assert!(read_page.verify_checksum());
    }

    #[test]
    fn test_page_checksum_mismatch_detection() {
        let temp_file = NamedTempFile::new().unwrap();
        let path = temp_file.path().to_path_buf();

        let mut pm = PageManager::open(&path).unwrap();
        let mut page = pm.allocate_page(PageType::Data, 200).unwrap();
        page.data[0..5].copy_from_slice(b"hello");
        pm.write_page(&mut page).unwrap();

        // Direct corruption of page bytes on disk
        let mut f = OpenOptions::new().write(true).open(&path).unwrap();
        f.seek(SeekFrom::Start(HEADER_SIZE as u64 + 1)).unwrap();
        f.write_all(b"CORRUPT").unwrap();
        f.flush().unwrap();

        // Reading corrupt page must fail with checksum error
        let res = pm.read_page(page.header.page_id);
        assert!(res.is_err());
        assert!(res.unwrap_err().contains("checksum mismatch"));
    }

    #[test]
    fn test_free_page_reallocation() {
        let temp_file = NamedTempFile::new().unwrap();
        let path = temp_file.path().to_path_buf();

        let mut pm = PageManager::open(&path).unwrap();
        let page1 = pm.allocate_page(PageType::Data, 1).unwrap();
        let _page2 = pm.allocate_page(PageType::Data, 2).unwrap();

        assert_eq!(pm.get_free_page_count(), 0);

        pm.free_page(page1.header.page_id).unwrap();
        assert_eq!(pm.get_free_page_count(), 1);

        let realloc_page = pm.allocate_page(PageType::Index, 3).unwrap();
        assert_eq!(realloc_page.header.page_id, page1.header.page_id);
        assert_eq!(pm.get_free_page_count(), 0);
    }
}

