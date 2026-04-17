import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItemButton, ListItemText, TextField,
  Snackbar, Alert, CircularProgress, Paper,
  Dialog, DialogTitle, DialogContent, DialogActions,
  FormControlLabel, Switch, Tooltip
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import SaveIcon from '@mui/icons-material/Save';
import RefreshIcon from '@mui/icons-material/Refresh';
import { useProjectContext } from '../contexts/ProjectContext';

type Document = {
  name: string;
  fields: Record<string, any>;
  createTime: string;
  updateTime: string;
};

type FirestoreManagerDrawerProps = { open: boolean; onClose: () => void };

export default function FirestoreManagerDrawer({ open, onClose }: FirestoreManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [collections, setCollections] = useState<string[]>([]);
  const [activeCollection, setActiveCollection] = useState<string | null>(null);
  
  const [documents, setDocuments] = useState<Document[]>([]);
  const [activeDocument, setActiveDocument] = useState<Document | null>(null);
  
  const [jsonText, setJsonText] = useState('');
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });

  // Dynamic SDK Live Sync feature
  const [liveSync, setLiveSync] = useState(false);
  const [firebaseDb, setFirebaseDb] = useState<any>(null);

  // Dialog for new collection/doc
  const [newColOpen, setNewColOpen] = useState(false);
  const [newColName, setNewColName] = useState('');
  
  const dbRoot = `/api/manage/firestore/projects/${activeProject}/databases/(default)/documents`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  useEffect(() => {
    if (liveSync && !firebaseDb) {
      setLoading(true);
      (async () => {
        try {
          // @ts-ignore
          const { initializeApp } = await import('https://www.gstatic.com/firebasejs/10.7.1/firebase-app.js');
          // @ts-ignore
          const { getFirestore, connectFirestoreEmulator } = await import('https://www.gstatic.com/firebasejs/10.7.1/firebase-firestore.js');
          
          const app = initializeApp({ projectId: activeProject });
          const db = getFirestore(app);
          // 8080 routes all proxy traffic in minisky
          connectFirestoreEmulator(db, window.location.hostname, 8080, { mockUserToken: "foo" });
          setFirebaseDb(db);
          showToast('Dynamic Firebase SDK Injected & Initialized');
        } catch (err: any) {
          showToast('Failed to load SDK dynamically: ' + err.message, 'error');
          setLiveSync(false);
        } finally {
          setLoading(false);
        }
      })();
    }
  }, [liveSync, firebaseDb, activeProject]);

  const loadCollections = useCallback(async () => {
    try {
      const res = await fetch(`${dbRoot}:listCollectionIds`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ pageSize: 100 })
      });
      if (res.ok) {
        const data = await res.json();
        setCollections(data.collectionIds || []);
      } else if (res.status === 404) {
        // Database might not be initialized yet via actual documents
        setCollections([]);
      }
    } catch (e) {
      console.error(e);
    }
  }, [dbRoot]);

  const loadDocuments = useCallback(async (col: string) => {
    setLoading(true);
    try {
      const res = await fetch(`${dbRoot}/${col}`);
      if (res.ok) {
        const data = await res.json();
        setDocuments(data.documents || []);
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [dbRoot]);

  useEffect(() => {
    if (open) {
      loadCollections();
    } else {
      setActiveCollection(null);
      setActiveDocument(null);
    }
  }, [open, loadCollections]);

  // When collection changes, load docs
  useEffect(() => {
    let unsub: any = null;
    
    if (activeCollection) {
      if (liveSync && firebaseDb) {
        setLoading(true);
        (async () => {
          // @ts-ignore
          const { collection, onSnapshot } = await import('https://www.gstatic.com/firebasejs/10.7.1/firebase-firestore.js');
          const colRef = collection(firebaseDb, activeCollection);
          unsub = onSnapshot(colRef, (snapshot: any) => {
            const docs: Document[] = [];
            snapshot.forEach((docSnap: any) => {
              // Convert SDK doc format locally
              docs.push({
                name: `projects/${activeProject}/databases/(default)/documents/${activeCollection}/${docSnap.id}`,
                fields: docSnap.data(), // Show Raw SDK objects natively in editor
                createTime: new Date().toISOString(),
                updateTime: new Date().toISOString()
              });
            });
            setDocuments(docs);
            setLoading(false);
          });
        })();
      } else {
        loadDocuments(activeCollection);
      }
      setActiveDocument(null);
    } else {
      setDocuments([]);
    }

    return () => {
      if (unsub) unsub();
    };
  }, [activeCollection, loadDocuments, liveSync, firebaseDb, activeProject]);

  // When active document changes, format JSON
  useEffect(() => {
    if (activeDocument) {
      setJsonText(JSON.stringify(activeDocument.fields || {}, null, 2));
    } else {
      setJsonText('');
    }
  }, [activeDocument]);

  const handleCreateDocument = async () => {
    if (!activeCollection) return;
    try {
      const res = await fetch(`${dbRoot}/${activeCollection}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ fields: {} })
      });
      if (res.ok) {
        showToast('Created new document');
        loadDocuments(activeCollection);
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed', 'error');
      }
    } catch (e: any) {
      showToast(e.message, 'error');
    }
  };

  const handleSaveDocument = async () => {
    if (!activeDocument) return;
    try {
      let fields = {};
      if (jsonText.trim() !== '') {
        fields = JSON.parse(jsonText);
      }
      
      const docPath = activeDocument.name; // full uri
      const docId = docPath.split('/').pop();
      const res = await fetch(`${dbRoot}/${activeCollection}/${docId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ fields })
      });
      
      if (res.ok) {
        showToast('Document saved successfully');
        loadDocuments(activeCollection!);
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Save failed', 'error');
      }
    } catch (e: any) {
      showToast('Invalid JSON format: ' + e.message, 'error');
    }
  };

  const handleDeleteDocument = async (docName: string) => {
    const docId = docName.split('/').pop();
    await fetch(`${dbRoot}/${activeCollection}/${docId}`, { method: 'DELETE' });
    showToast('Document deleted');
    if (activeDocument?.name === docName) {
      setActiveDocument(null);
    }
    loadDocuments(activeCollection!);
    // Refresh collections if creating/deleting the only doc in a collection
    loadCollections();
  };

  const handleCreateCollection = () => {
    if (!newColName) return;
    // Firestore creates a collection only when a document is added.
    // So we add it locally and let the user add a document to physically persist it.
    if (!collections.includes(newColName)) {
      setCollections([...collections, newColName]);
    }
    setActiveCollection(newColName);
    setNewColOpen(false);
    setNewColName('');
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '85vw', maxWidth: 1200, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#1a73e8' }}>Cloud Firestore</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Data Explorer • {activeProject}</Typography>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center' }}>
              <Tooltip title="Dynamically loads Firestore Web SDK via UI to stream WebSockets from the Emulator">
                <FormControlLabel
                  control={<Switch size="small" checked={liveSync} onChange={(e) => setLiveSync(e.target.checked)} />}
                  label={<Typography variant="caption" sx={{ fontWeight: 600, mr: 2, color: liveSync ? 'success.main' : 'text.secondary' }}>Live SDK Sync</Typography>}
                  sx={{ margin: 0 }}
                />
              </Tooltip>
              <IconButton onClick={() => loadCollections()} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
            {/* Left Pane - Collections */}
            <Box sx={{ width: 280, borderRight: '1px solid #dadce0', bgcolor: 'white', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ p: 1.5, borderBottom: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Collections</Typography>
                <IconButton size="small" onClick={() => setNewColOpen(true)}><AddIcon fontSize="small" /></IconButton>
              </Box>
              <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                {collections.map(col => (
                  <ListItemButton 
                    key={col} 
                    selected={activeCollection === col}
                    onClick={() => setActiveCollection(col)}
                    sx={{ borderBottom: '1px solid #f1f3f4', py: 1.5 }}
                  >
                    <ListItemText primary={<Typography sx={{ fontSize: '0.85rem', fontWeight: activeCollection === col ? 600 : 400 }}>{col}</Typography>} />
                  </ListItemButton>
                ))}
              </List>
            </Box>

            {/* Middle Pane - Documents */}
            <Box sx={{ width: 320, borderRight: '1px solid #dadce0', bgcolor: '#fff', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ p: 1.5, borderBottom: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Documents</Typography>
                <IconButton size="small" onClick={handleCreateDocument} disabled={!activeCollection} color="primary">
                  <AddIcon fontSize="small" />
                </IconButton>
              </Box>
              
              {loading && <Box sx={{ p: 2, textAlign: 'center' }}><CircularProgress size={24} /></Box>}
              
              <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                {!loading && documents.length === 0 && activeCollection && (
                  <Typography variant="caption" sx={{ display: 'block', p: 2, color: '#80868b', fontStyle: 'italic' }}>
                    No documents. Collection will not persist until a document is created.
                  </Typography>
                )}
                {documents.map(doc => {
                  const docId = doc.name.split('/').pop();
                  return (
                    <ListItemButton 
                      key={doc.name} 
                      selected={activeDocument?.name === doc.name}
                      onClick={() => setActiveDocument(doc)}
                      sx={{ borderBottom: '1px solid #f1f3f4', py: 1.5 }}
                    >
                      <ListItemText 
                        primary={<Typography sx={{ fontSize: '0.85rem', fontFamily: 'monospace', fontWeight: activeDocument?.name === doc.name ? 600 : 400 }}>{docId}</Typography>} 
                        secondary={<Typography sx={{ fontSize: '0.7rem' }}>Updated: {new Date(doc.updateTime).toLocaleString()}</Typography>}
                      />
                      <IconButton size="small" sx={{ opacity: 0.6, '&:hover': { opacity: 1, color: 'error.main' } }} onClick={(e) => { e.stopPropagation(); handleDeleteDocument(doc.name); }}>
                        <DeleteIcon fontSize="inherit" />
                      </IconButton>
                    </ListItemButton>
                  );
                })}
              </List>
            </Box>

            {/* Right Pane - Editor */}
            <Box sx={{ flex: 1, p: 3, display: 'flex', flexDirection: 'column', bgcolor: '#f8f9fa' }}>
              {activeDocument ? (
                <>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                    <Typography variant="subtitle1" sx={{ fontFamily: 'monospace', fontWeight: 600 }}>
                      {activeDocument.name.split('/').pop()}
                    </Typography>
                    <Button variant="contained" size="small" startIcon={<SaveIcon />} onClick={handleSaveDocument}>
                      Save Fields
                    </Button>
                  </Box>
                  <Typography variant="caption" sx={{ color: '#5f6368', mb: 1, display: 'block' }}>
                    Edit the Firestore REST API `fields` payload natively:
                  </Typography>
                  <Paper sx={{ flex: 1, display: 'flex' }} elevation={0}>
                    <TextField
                      multiline
                      fullWidth
                      variant="outlined"
                      value={jsonText}
                      onChange={e => setJsonText(e.target.value)}
                      sx={{ height: '100%', '& .MuiInputBase-root': { height: '100%', alignItems: 'flex-start', fontFamily: 'monospace', fontSize: '0.85rem' } }}
                    />
                  </Paper>
                </>
              ) : (
                <Box sx={{ display: 'flex', height: '100%', alignItems: 'center', justifyContent: 'center', color: '#80868b' }}>
                  <Typography variant="body2">Select a document to edit its fields</Typography>
                </Box>
              )}
            </Box>
          </Box>
        </Box>
      </Drawer>

      <Dialog open={newColOpen} onClose={() => setNewColOpen(false)}>
        <DialogTitle>Start a Collection</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Collection ID" fullWidth variant="standard"
                     value={newColName} onChange={e => setNewColName(e.target.value.replace(/[^a-zA-Z0-9_-]/g, ''))} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewColOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateCollection} disabled={!newColName}>Create</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
