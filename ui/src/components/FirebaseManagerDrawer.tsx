import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItem, ListItemText, TextField,
  Snackbar, Alert, CircularProgress, Paper,
  Tabs, Tab, Stack,
  Dialog, DialogTitle, DialogContent, DialogActions
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import SaveIcon from '@mui/icons-material/Save';
import PersonAddIcon from '@mui/icons-material/PersonAdd';
import StorageIcon from '@mui/icons-material/Storage';
import VpnKeyIcon from '@mui/icons-material/VpnKey';
import { useProjectContext } from '../contexts/ProjectContext';

type User = {
  localId: string;
  email: string;
  displayName?: string;
  createdAt: string;
  lastLoginAt: string;
};

type FirebaseManagerDrawerProps = { open: boolean; onClose: () => void };

export default function FirebaseManagerDrawer({ open, onClose }: FirebaseManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [tab, setTab] = useState(0);
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });

  // Auth State
  const [users, setUsers] = useState<User[]>([]);
  const [newUserEmail, setNewUserEmail] = useState('');
  const [newUserPass, setNewUserPass] = useState('');
  const [showAddUser, setShowAddUser] = useState(false);

  // RTDB State
  const [dbPath, setDbPath] = useState('/');
  const [jsonEdit, setJsonEdit] = useState('');

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  // --- Auth Logic ---
  const loadUsers = useCallback(async () => {
    setLoading(true);
    try {
      // Identity Toolkit List Users Endpoint
      const res = await fetch(`/api/manage/firebase/identitytoolkit.googleapis.com/v1/projects/${activeProject}/accounts:batchGet?maxResults=100`);
      if (res.ok) {
        const data = await res.json();
        setUsers(data.users || []);
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [activeProject]);

  const handleAddUser = async () => {
    try {
      const res = await fetch(`/api/manage/firebase/identitytoolkit.googleapis.com/v1/projects/${activeProject}/accounts`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: newUserEmail,
          password: newUserPass,
          emailVerified: true
        })
      });
      if (res.ok) {
        showToast('User created successfully');
        loadUsers();
        setShowAddUser(false);
        setNewUserEmail('');
        setNewUserPass('');
      } else {
        const err = await res.json();
        showToast(err.error?.message || 'Failed to create user', 'error');
      }
    } catch (e: any) {
      showToast(e.message, 'error');
    }
  };

  const handleDeleteUser = async (uid: string) => {
    try {
      const res = await fetch(`/api/manage/firebase/identitytoolkit.googleapis.com/v1/projects/${activeProject}/accounts:delete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ localId: uid })
      });
      if (res.ok) {
        showToast('User deleted');
        loadUsers();
      }
    } catch (e: any) {
      showToast(e.message, 'error');
    }
  };

  // --- RTDB Logic ---
  const loadRtdb = useCallback(async (path: string = '/') => {
    setLoading(true);
    try {
      const cleanPath = path.endsWith('.json') ? path : `${path}.json`;
      const res = await fetch(`/api/manage/firebase/firebaseio.com${cleanPath}?ns=${activeProject}`);
      if (res.ok) {
        const data = await res.json();
        setJsonEdit(JSON.stringify(data, null, 2));
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [activeProject]);

  const handleSaveRtdb = async () => {
    try {
      const parsed = JSON.parse(jsonEdit);
      const cleanPath = dbPath.endsWith('.json') ? dbPath : `${dbPath}.json`;
      const res = await fetch(`/api/manage/firebase/firebaseio.com${cleanPath}?ns=${activeProject}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(parsed)
      });
      if (res.ok) {
        showToast('Database updated');
        loadRtdb(dbPath);
      }
    } catch (e: any) {
      showToast('Invalid JSON: ' + e.message, 'error');
    }
  };

  useEffect(() => {
    if (open) {
      if (tab === 0) loadUsers();
      if (tab === 1) loadRtdb(dbPath);
    }
  }, [open, tab, loadUsers, loadRtdb, dbPath]);

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ width: '80vw', maxWidth: 1000, height: '100vh', display: 'flex', flexDirection: 'column', bgcolor: '#f5f7f9' }}>
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #e0e0e0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 600, color: '#f57c00', display: 'flex', alignItems: 'center', gap: 1 }}>
                <StorageIcon color="warning" /> Firebase Manager
              </Typography>
              <Typography variant="caption" sx={{ color: 'text.secondary' }}>Project: {activeProject}</Typography>
            </Box>
            <IconButton onClick={onClose}><CloseIcon /></IconButton>
          </Box>

          <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ bgcolor: 'white', borderBottom: '1px solid #e0e0e0' }}>
            <Tab icon={<VpnKeyIcon sx={{ fontSize: '1.2rem' }} />} iconPosition="start" label="Authentication" />
            <Tab icon={<StorageIcon sx={{ fontSize: '1.2rem' }} />} iconPosition="start" label="Realtime Database" />
          </Tabs>

          <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
            {tab === 0 && (
              <Box>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                  <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Users</Typography>
                  <Stack direction="row" spacing={1}>
                    <IconButton onClick={loadUsers} size="small"><RefreshIcon /></IconButton>
                    <Button variant="contained" color="warning" size="small" startIcon={<PersonAddIcon />} onClick={() => setShowAddUser(true)}>
                      Add User
                    </Button>
                  </Stack>
                </Box>
                
                <Paper sx={{ width: '100%' }}>
                  {loading && <Box sx={{ p: 4, textAlign: 'center' }}><CircularProgress color="warning" /></Box>}
                  <List>
                    {!loading && users.length === 0 && <ListItem><ListItemText secondary="No users found" /></ListItem>}
                    {users.map(u => (
                      <ListItem key={u.localId} divider sx={{ '&:hover': { bgcolor: '#fafafa' } }}>
                        <ListItemText 
                          primary={u.email} 
                          secondary={`UID: ${u.localId} • Created: ${new Date(parseInt(u.createdAt)).toLocaleString()}`} 
                        />
                        <IconButton size="small" color="error" onClick={() => handleDeleteUser(u.localId)}><DeleteIcon fontSize="small" /></IconButton>
                      </ListItem>
                    ))}
                  </List>
                </Paper>
              </Box>
            )}

            {tab === 1 && (
              <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                <Box sx={{ display: 'flex', gap: 2, mb: 2, alignItems: 'center' }}>
                  <TextField 
                    label="DB Path" 
                    size="small" 
                    value={dbPath} 
                    onChange={e => setDbPath(e.target.value)} 
                    placeholder="/users/123"
                    sx={{ flex: 1 }}
                  />
                  <IconButton onClick={() => loadRtdb(dbPath)} size="small"><RefreshIcon /></IconButton>
                  <Button variant="contained" color="warning" startIcon={<SaveIcon />} onClick={handleSaveRtdb}>
                    Save
                  </Button>
                </Box>

                <Paper sx={{ flex: 1, display: 'flex', p: 1, bgcolor: '#1e1e1e' }}>
                  <TextField
                    multiline
                    fullWidth
                    value={jsonEdit}
                    onChange={e => setJsonEdit(e.target.value)}
                    variant="outlined"
                    sx={{
                      '& .MuiInputBase-root': {
                        color: '#d4d4d4',
                        fontFamily: 'monospace',
                        fontSize: '0.9rem',
                        height: '100%',
                        alignItems: 'flex-start'
                      }
                    }}
                  />
                </Paper>
              </Box>
            )}
          </Box>
        </Box>
      </Drawer>

      <Dialog open={showAddUser} onClose={() => setShowAddUser(false)}>
        <DialogTitle>Add New Firebase User</DialogTitle>
        <DialogContent sx={{ minWidth: 300 }}>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <TextField label="Email" fullWidth value={newUserEmail} onChange={e => setNewUserEmail(e.target.value)} />
            <TextField label="Password" type="password" fullWidth value={newUserPass} onChange={e => setNewUserPass(e.target.value)} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowAddUser(false)}>Cancel</Button>
          <Button onClick={handleAddUser} variant="contained" color="warning" disabled={!newUserEmail || !newUserPass}>Create</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
