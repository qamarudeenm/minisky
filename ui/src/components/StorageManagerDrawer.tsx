import {
  Drawer, Box, Typography, IconButton, Button,
  Table, TableBody, TableCell, TableHead, TableRow,
  TextField, Breadcrumbs, Link
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import CreateNewFolderIcon from '@mui/icons-material/CreateNewFolder';
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile';
import { useState, useEffect } from 'react';
import { useProjectContext } from '../contexts/ProjectContext';

type StorageManagerDrawerProps = {
  open: boolean;
  onClose: () => void;
};

export default function StorageManagerDrawer({ open, onClose }: StorageManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [buckets, setBuckets] = useState<any[]>([]);
  const [newBucketName, setNewBucketName] = useState('');
  
  const [currentBucket, setCurrentBucket] = useState<string | null>(null);
  const [objects, setObjects] = useState<any[]>([]);
  const [uploadFile, setUploadFile] = useState<File | null>(null);

  const loadBuckets = async () => {
    try {
      const res = await fetch(`/api/manage/storage/b?project=${activeProject}`);
      if (res.ok) {
        const data = await res.json();
        setBuckets(data.items || []);
      }
    } catch (e) {
      console.error(e);
    }
  };

  const loadObjects = async (bucket: string) => {
    try {
      const res = await fetch(`/api/manage/storage/b/${bucket}/o`);
      if (res.ok) {
        const data = await res.json();
        setObjects(data.items || []);
      }
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => {
    if (open) {
      loadBuckets();
      setCurrentBucket(null);
    }
  }, [open, activeProject]);

  useEffect(() => {
    if (currentBucket) {
      loadObjects(currentBucket);
    }
  }, [currentBucket]);

  const handleCreateBucket = async () => {
    if (!newBucketName) return;
    await fetch(`/api/manage/storage/b?project=${activeProject}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: newBucketName })
    });
    setNewBucketName('');
    loadBuckets();
  };

  const handleDeleteBucket = async (name: string) => {
    if (!confirm(`Delete bucket "${name}" and all contents?`)) return;
    await fetch(`/api/manage/storage/b/${name}`, { method: 'DELETE' });
    loadBuckets();
  };

  const handleUpload = async () => {
    if (!uploadFile || !currentBucket) return;
    const formData = new FormData();
    formData.append('file', uploadFile);
    await fetch(`/api/manage/storage/b/${currentBucket}/o?name=${encodeURIComponent(uploadFile.name)}`, {
      method: 'POST',
      body: formData
    });
    setUploadFile(null);
    loadObjects(currentBucket);
  };

  const handleDeleteObject = async (name: string) => {
    if (!currentBucket) return;
    await fetch(`/api/manage/storage/b/${currentBucket}/o/${encodeURIComponent(name)}`, { method: 'DELETE' });
    loadObjects(currentBucket);
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: '600px', p: 3 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h5" sx={{ fontWeight: 500 }}>Cloud Storage Manager</Typography>
        <IconButton onClick={onClose}><CloseIcon /></IconButton>
      </Box>

      {currentBucket ? (
        <Box>
          <Breadcrumbs sx={{ mb: 3 }}>
            <Link component="button" variant="body1" onClick={() => setCurrentBucket(null)}>Buckets</Link>
            <Typography variant="body1" color="text.primary">{currentBucket}</Typography>
          </Breadcrumbs>
          
          <Box sx={{ display: 'flex', gap: 2, mb: 3, alignItems: 'center' }}>
            <input type="file" onChange={e => setUploadFile(e.target.files?.[0] || null)} />
            <Button variant="contained" onClick={handleUpload} disabled={!uploadFile}>Upload Object</Button>
          </Box>

          <Table size="small">
            <TableHead><TableRow><TableCell>Name</TableCell><TableCell>Size</TableCell><TableCell>Actions</TableCell></TableRow></TableHead>
            <TableBody>
              {objects.length === 0 && <TableRow><TableCell colSpan={3} align="center">No objects found</TableCell></TableRow>}
              {objects.map(o => (
                <TableRow key={o.name}>
                  <TableCell><Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}><InsertDriveFileIcon fontSize="small"/> {o.name}</Box></TableCell>
                  <TableCell>{Math.round(parseInt(o.size || '0') / 1024)} KB</TableCell>
                  <TableCell>
                    <IconButton size="small" color="error" onClick={() => handleDeleteObject(o.name)}><DeleteIcon fontSize="small"/></IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      ) : (
        <Box>
          <Box sx={{ display: 'flex', gap: 2, mb: 3 }}>
            <TextField size="small" label="New Bucket Name" value={newBucketName} onChange={e => setNewBucketName(e.target.value)} fullWidth />
            <Button variant="contained" onClick={handleCreateBucket}>Create</Button>
          </Box>

          <Table size="small">
            <TableHead><TableRow><TableCell>Bucket Name</TableCell><TableCell align="right">Actions</TableCell></TableRow></TableHead>
            <TableBody>
              {buckets.length === 0 && <TableRow><TableCell colSpan={2} align="center">No buckets found</TableCell></TableRow>}
              {buckets.map(b => (
                <TableRow key={b.name} hover sx={{ cursor: 'pointer' }} onClick={() => setCurrentBucket(b.name)}>
                  <TableCell><Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}><CreateNewFolderIcon fontSize="small"/> {b.name}</Box></TableCell>
                  <TableCell align="right">
                    <IconButton size="small" color="error" onClick={(e) => { e.stopPropagation(); handleDeleteBucket(b.name); }}><DeleteIcon fontSize="small"/></IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      )}
      </Box>
    </Drawer>
  );
}
