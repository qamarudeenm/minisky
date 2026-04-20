import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItemButton, ListItemText,
  Snackbar, Alert, Dialog,
  DialogTitle, DialogContent, DialogActions, Paper,
  FormControl, InputLabel, Select, MenuItem, TextField, Chip
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useProjectContext } from '../contexts/ProjectContext';

type CloudSqlManagerDrawerProps = { open: boolean; onClose: () => void };

export default function CloudSqlManagerDrawer({ open, onClose }: CloudSqlManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [instances, setInstances] = useState<any[]>([]);
  const [activeInstance, setActiveInstance] = useState<any | null>(null);

  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });

  // Dialogs
  const [newInstanceOpen, setNewInstanceOpen] = useState(false);
  const [newInstanceName, setNewInstanceName] = useState('');
  const [newDbVersion, setNewDbVersion] = useState('POSTGRES_18');
  const [availableDbVersions, setAvailableDbVersions] = useState<any[]>([]);

  const apiRoot = `/api/manage/cloudsql/projects/${activeProject}`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadInstances = useCallback(async () => {
    try {
      const res = await fetch(`${apiRoot}/instances`);
      if (res.ok) {
        const data = await res.json();
        setInstances(data.items || []);
      }
    } catch (e) { console.error(e); }
  }, [apiRoot]);

  useEffect(() => {
    if (open) {
      loadInstances();
      fetch('/api/config/images')
        .then(r => r.json())
        .then(d => {
          const pg = (d.sql?.postgres?.versions || []).map((v: any) => ({ ...v, engine: 'POSTGRES' }));
          const my = (d.sql?.mysql?.versions || []).map((v: any) => ({ ...v, engine: 'MYSQL' }));
          setAvailableDbVersions([...pg, ...my]);
        })
        .catch(console.error);

      // Start polling for status changes
      const t = setInterval(loadInstances, 3000);
      return () => clearInterval(t);
    } else {
      setActiveInstance(null);
    }
  }, [open, loadInstances]);

  // Keep activeInstance in sync with the updated instances list
  useEffect(() => {
    if (activeInstance) {
      const updated = instances.find(i => i.name === activeInstance.name);
      if (updated && (updated.state !== activeInstance.state || updated.ipAddresses?.length !== activeInstance.ipAddresses?.length)) {
        setActiveInstance(updated);
      }
    }
  }, [instances, activeInstance]);

  const handleCreateInstance = async () => {
    if (!newInstanceName) return;
    try {
      const payload = {
        name: newInstanceName,
        databaseVersion: newDbVersion,
        region: "us-central1"
      };
      const res = await fetch(`${apiRoot}/instances`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        showToast('Processing Database Instance Creation...');
        setTimeout(loadInstances, 1000); // Wait for the state to bounce
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewInstanceOpen(false);
    setNewInstanceName('');
  };

  const handleDeleteInstance = async (name: string) => {
    await fetch(`${apiRoot}/instances/${name}`, { method: 'DELETE' });
    if (activeInstance?.name === name) setActiveInstance(null);
    showToast('Tearing down Database...');
    setTimeout(loadInstances, 1000);
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    showToast('Copied to clipboard');
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '75vw', maxWidth: 1000, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#1a73e8' }}>Cloud SQL</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Database Instances mapped to Local Docker Volumes • {activeProject}</Typography>
            </Box>
            <Box>
              <IconButton onClick={() => loadInstances()} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
            {/* Left Pane - Instances */}
            <Box sx={{ width: 280, borderRight: '1px solid #dadce0', bgcolor: 'white', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ p: 1.5, borderBottom: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Instances</Typography>
                <IconButton size="small" onClick={() => setNewInstanceOpen(true)}><AddIcon fontSize="small" /></IconButton>
              </Box>
              <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                {instances.map(inst => (
                  <ListItemButton 
                    key={inst.name} 
                    selected={activeInstance?.name === inst.name}
                    onClick={() => setActiveInstance(inst)}
                    sx={{ borderBottom: '1px solid #f1f3f4', py: 1.5 }}
                  >
                    <ListItemText 
                      primary={<Typography sx={{ fontSize: '0.85rem', fontWeight: activeInstance?.name === inst.name ? 600 : 400 }}>{inst.name}</Typography>} 
                      secondary={<Typography variant="caption" sx={{ color: inst.state === 'RUNNABLE' ? 'success.main' : 'warning.main' }}>{inst.state}</Typography>}
                    />
                    <IconButton size="small" sx={{ opacity: 0.6, '&:hover': { opacity: 1, color: 'error.main' } }} onClick={(e) => { e.stopPropagation(); handleDeleteInstance(inst.name); }}>
                      <DeleteIcon fontSize="inherit" />
                    </IconButton>
                  </ListItemButton>
                ))}
              </List>
            </Box>

            {/* Right Pane - Details */}
            <Box sx={{ flex: 1, p: 4, display: 'flex', flexDirection: 'column', bgcolor: '#f8f9fa', overflow: 'auto' }}>
              {!activeInstance ? (
                 <Typography variant="body2" sx={{ color: '#80868b', fontStyle: 'italic', textAlign: 'center', mt: 10 }}>
                   Select a Database Instance to view its connection credentials.
                 </Typography>
              ) : (
                <Box>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 3 }}>
                     <Box>
                       <Typography variant="h5" sx={{ fontWeight: 500, mb: 0.5 }}>{activeInstance.name}</Typography>
                       <Chip label={activeInstance.databaseVersion} size="small" sx={{ mr: 1, fontWeight: 600 }} />
                       <Chip label={activeInstance.state} color={activeInstance.state === 'RUNNABLE' ? 'success' : 'warning'} size="small" variant="outlined" />
                     </Box>
                  </Box>

                  <Paper sx={{ p: 3, mb: 3, border: '1px solid #dadce0' }} elevation={0}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 2, color: '#5f6368' }}>Connection Details</Typography>
                    
                    <Box sx={{ display: 'grid', gridTemplateColumns: '120px 1fr auto', gap: 2, alignItems: 'center', mb: 1.5 }}>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>Connection Name:</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{activeInstance.connectionName}</Typography>
                      <IconButton size="small" onClick={() => copyToClipboard(activeInstance.connectionName)}><ContentCopyIcon fontSize="small" /></IconButton>
                    </Box>

                    {activeInstance.ipAddresses && activeInstance.ipAddresses.map((ip: any, idx: number) => (
                      <Box key={idx} sx={{ display: 'grid', gridTemplateColumns: '120px 1fr auto', gap: 2, alignItems: 'center', mb: 1.5 }}>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>IP Address:</Typography>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{ip.ipAddress}</Typography>
                        <IconButton size="small" onClick={() => copyToClipboard(ip.ipAddress)}><ContentCopyIcon fontSize="small" /></IconButton>
                      </Box>
                    ))}

                    <Box sx={{ display: 'grid', gridTemplateColumns: '120px 1fr auto', gap: 2, alignItems: 'center', mb: 1.5 }}>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>Root User:</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                        {activeInstance.databaseVersion.startsWith('POSTGRES') ? 'postgres' : 'root'}
                      </Typography>
                      <IconButton size="small" onClick={() => copyToClipboard(activeInstance.databaseVersion.startsWith('POSTGRES') ? 'postgres' : 'root')}><ContentCopyIcon fontSize="small" /></IconButton>
                    </Box>

                    <Box sx={{ display: 'grid', gridTemplateColumns: '120px 1fr auto', gap: 2, alignItems: 'center' }}>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>Root Password:</Typography>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>minisky</Typography>
                      <IconButton size="small" onClick={() => copyToClipboard('minisky')}><ContentCopyIcon fontSize="small" /></IconButton>
                    </Box>
                  </Paper>

                  <Paper sx={{ p: 3, border: '1px solid #dadce0', bgcolor: '#f1f3f4' }} elevation={0}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>How to Connect</Typography>
                    <Typography variant="body2" sx={{ mb: 2, color: '#3c4043' }}>
                      The Docker container for this database is fully bridged to your host machine over TCP. You can connect using your local IDE (DataGrip, DBeaver) or CLI clients using the credentials above.
                    </Typography>
                    <Typography variant="caption" sx={{ display: 'block', fontFamily: 'monospace', p: 1.5, bgcolor: '#e8eaed', borderRadius: 1 }}>
                      {activeInstance.databaseVersion.startsWith('POSTGRES') 
                        ? `psql postgresql://postgres:minisky@${activeInstance.ipAddresses?.[0]?.ipAddress}/postgres`
                        : `mysql -h ${activeInstance.ipAddresses?.[0]?.ipAddress?.split(':')[0]} -P ${activeInstance.ipAddresses?.[0]?.ipAddress?.split(':')[1] || '3306'} -u root -pminisky`
                      }
                    </Typography>
                  </Paper>

                </Box>
              )}
            </Box>
          </Box>
        </Box>
      </Drawer>

      <Dialog open={newInstanceOpen} onClose={() => setNewInstanceOpen(false)}>
        <DialogTitle>Create SQL Instance</DialogTitle>
        <DialogContent sx={{ width: 400 }}>
          <TextField autoFocus margin="dense" label="Instance Name" fullWidth variant="outlined"
                     value={newInstanceName} onChange={e => setNewInstanceName(e.target.value.replace(/[^a-zA-Z0-9-]/g, ''))} sx={{ mb: 3, mt: 1 }} />
          
          <FormControl fullWidth>
            <InputLabel>Database Version</InputLabel>
            <Select
              value={newDbVersion}
              label="Database Version"
              onChange={(e) => setNewDbVersion(e.target.value)}
            >
              {availableDbVersions.map(v => (
                <MenuItem key={`${v.engine}_${v.version}`} value={`${v.engine}_${v.version.replace('.', '_')}`}>
                  {v.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </DialogContent>
        <DialogActions sx={{ p: 2, pt: 0 }}>
          <Button onClick={() => setNewInstanceOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={handleCreateInstance} disabled={!newInstanceName}>Provision Container</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
