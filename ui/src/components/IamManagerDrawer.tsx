import { useState, useEffect } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  Table, TableBody, TableCell, TableHead, TableRow,
  TextField, Breadcrumbs, Link, Chip, Tooltip, Snackbar, Alert
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import KeyIcon from '@mui/icons-material/VpnKey';
import PersonIcon from '@mui/icons-material/Person';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useProjectContext } from '../contexts/ProjectContext';

type IamManagerDrawerProps = {
  open: boolean;
  onClose: () => void;
};

export default function IamManagerDrawer({ open, onClose }: IamManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [serviceAccounts, setServiceAccounts] = useState<any[]>([]);
  const [newAccountId, setNewAccountId] = useState('');
  
  const [currentSA, setCurrentSA] = useState<any | null>(null);
  const [keys, setKeys] = useState<any[]>([]);
  const [toast, setToast] = useState<{msg: string, open: boolean}>({msg: '', open: false});

  const loadServiceAccounts = async () => {
    try {
      const res = await fetch(`/api/manage/iam/projects/${activeProject}/serviceAccounts`);
      if (res.ok) {
        const data = await res.json();
        setServiceAccounts(data.accounts || []);
      }
    } catch (e) {
      console.error(e);
    }
  };

  const loadKeys = async (email: string) => {
    try {
      const res = await fetch(`/api/manage/iam/projects/${activeProject}/serviceAccounts/${email}/keys`);
      if (res.ok) {
        const data = await res.json();
        setKeys(data.keys || []);
      }
    } catch (e) {
      console.error(e);
    }
  };

  useEffect(() => {
    if (open) {
      loadServiceAccounts();
      setCurrentSA(null);
    }
  }, [open, activeProject]);

  useEffect(() => {
    if (currentSA) {
      loadKeys(currentSA.email);
    }
  }, [currentSA, activeProject]);

  const handleCreateSA = async () => {
    if (!newAccountId) return;
    await fetch(`/api/manage/iam/projects/${activeProject}/serviceAccounts`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ 
        accountId: newAccountId,
        serviceAccount: { displayName: `${newAccountId} display`, description: 'Created via UI dashboard' } 
      })
    });
    setNewAccountId('');
    loadServiceAccounts();
  };

  const handleDeleteSA = async (email: string) => {
    await fetch(`/api/manage/iam/projects/${activeProject}/serviceAccounts/${email}`, { method: 'DELETE' });
    loadServiceAccounts();
  };

  const handleCreateKey = async () => {
    if (!currentSA) return;
    const res = await fetch(`/api/manage/iam/projects/${activeProject}/serviceAccounts/${currentSA.email}/keys`, {
      method: 'POST'
    });
    if (res.ok) {
      const keyData = await res.json();
      // Auto-trigger fake download of the JSON file
      const decodedKey = atob(keyData.privateKeyData);
      const blob = new Blob([decodedKey], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${activeProject}-${currentSA.email.split('@')[0]}-key.json`;
      a.click();
    }
    loadKeys(currentSA.email);
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setToast({msg: 'Copied to clipboard!', open: true});
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ width: '700px', p: 4 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
            <Typography variant="h5" sx={{ fontWeight: 500 }}>IAM Manager</Typography>
            <IconButton onClick={onClose}><CloseIcon /></IconButton>
          </Box>
          <Typography variant="body2" sx={{ color: '#5f6368', mb: 3 }}>
            Current Project Context: <Chip label={activeProject} size="small" sx={{ ml: 1, backgroundColor: '#e8f0fe', color: '#1a73e8', fontWeight: 600 }} />
          </Typography>

          {currentSA ? (
            <Box>
              <Breadcrumbs sx={{ mb: 3 }}>
                <Link component="button" variant="body1" onClick={() => setCurrentSA(null)}>Service Accounts</Link>
                <Typography variant="body1" color="text.primary">{currentSA.email}</Typography>
              </Breadcrumbs>
              
              <Box sx={{ display: 'flex', gap: 2, mb: 3, alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography variant="h6">API Keys</Typography>
                <Button variant="contained" onClick={handleCreateKey} startIcon={<KeyIcon />}>Generate JSON Key</Button>
              </Box>

              <Table size="small">
                <TableHead><TableRow><TableCell>Key ID</TableCell><TableCell>Creation Date</TableCell><TableCell>Algo</TableCell></TableRow></TableHead>
                <TableBody>
                  {keys.length === 0 && <TableRow><TableCell colSpan={3} align="center">No active keys for this account</TableCell></TableRow>}
                  {keys.map(k => {
                    const kid = k.name.split('/').pop();
                    return (
                      <TableRow key={k.name}>
                        <TableCell>
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                            <KeyIcon fontSize="small" sx={{ color: '#f9ab00' }}/> 
                            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{kid.substring(0, 10)}...</Typography>
                          </Box>
                        </TableCell>
                        <TableCell>{new Date(k.validAfterTime).toLocaleDateString()}</TableCell>
                        <TableCell><Chip size="small" label={k.keyAlgorithm} variant="outlined" /></TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </Box>
          ) : (
            <Box>
              <Box sx={{ display: 'flex', gap: 2, mb: 4, mt: 2 }}>
                <TextField 
                  size="small" 
                  label="Service Account ID" 
                  placeholder="my-service-account"
                  value={newAccountId} 
                  onChange={e => setNewAccountId(e.target.value)} 
                  fullWidth 
                />
                <Button variant="contained" onClick={handleCreateSA} sx={{ whiteSpace: 'nowrap' }}>Create SA</Button>
              </Box>

              <Table size="small">
                <TableHead><TableRow><TableCell>Email / Link</TableCell><TableCell align="right">Actions</TableCell></TableRow></TableHead>
                <TableBody>
                  {serviceAccounts.length === 0 && <TableRow><TableCell colSpan={2} align="center">No service accounts found</TableCell></TableRow>}
                  {serviceAccounts.map(sa => (
                    <TableRow key={sa.email} hover>
                      <TableCell>
                        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                           <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
                              <PersonIcon fontSize="small" sx={{ color: '#1a73e8' }}/> 
                              <Link component="button" sx={{ fontWeight: 500, fontFamily: 'monospace' }} onClick={() => setCurrentSA(sa)}>
                                {sa.email}
                              </Link>
                              <Tooltip title="Copy Email">
                                <IconButton size="small" onClick={() => copyToClipboard(sa.email)}>
                                  <ContentCopyIcon sx={{ fontSize: 14 }} />
                                </IconButton>
                              </Tooltip>
                           </Box>
                           <Typography variant="caption" sx={{ color: '#80868b' }}>Unique ID: {sa.uniqueId}</Typography>
                        </Box>
                      </TableCell>
                      <TableCell align="right">
                        <IconButton size="small" color="error" onClick={(e) => { e.stopPropagation(); handleDeleteSA(sa.email); }}><DeleteIcon fontSize="small"/></IconButton>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Box>
          )}
        </Box>
      </Drawer>
      <Snackbar 
        open={toast.open} 
        autoHideDuration={2000} 
        onClose={() => setToast({msg: '', open: false})}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert severity="success" sx={{ width: '100%' }}>
          {toast.msg}
        </Alert>
      </Snackbar>
    </>
  );
}
