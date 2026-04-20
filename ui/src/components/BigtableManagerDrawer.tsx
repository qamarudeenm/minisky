import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItemButton, ListItemText, TextField,
  Snackbar, Alert, Paper, Dialog,
  DialogTitle, DialogContent, DialogActions,
  Divider
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import RefreshIcon from '@mui/icons-material/Refresh';
import StorageIcon from '@mui/icons-material/Storage';
import TableChartIcon from '@mui/icons-material/TableChart';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import { useProjectContext } from '../contexts/ProjectContext';

type BigtableManagerDrawerProps = { open: boolean; onClose: () => void };

export default function BigtableManagerDrawer({ open, onClose }: BigtableManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  
  const [instances, setInstances] = useState<any[]>([]);
  const [tablesByInstance, setTablesByInstance] = useState<Record<string, any[]>>({});
  
  const [, setLoading] = useState(false);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });

  // Creation State
  const [newInstanceOpen, setNewInstanceOpen] = useState(false);
  const [newInstanceId, setNewInstanceId] = useState('');
  const [newInstanceName, setNewInstanceName] = useState('');
  
  const [newTableOpen, setNewTableOpen] = useState(false);
  const [targetInstance, setTargetInstance] = useState('');
  const [newTableId, setNewTableId] = useState('');

  const apiRoot = `/api/manage/bigtableadmin/projects/${activeProject}`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadInstances = useCallback(async () => {
    try {
      const res = await fetch(`${apiRoot}/instances`);
      if (res.ok) {
        const data = await res.json();
        const instList = data.instances || [];
        setInstances(instList);
        
        // Load tables for each instance
        const tbMap: Record<string, any[]> = {};
        for (const inst of instList) {
          const instId = inst.name.split('/').pop();
          const tbRes = await fetch(`${apiRoot}/instances/${instId}/tables`);
          if (tbRes.ok) {
            const tbData = await tbRes.json();
            tbMap[instId] = tbData.tables || [];
          }
        }
        setTablesByInstance(tbMap);
      }
    } catch (e) { console.error(e); }
  }, [apiRoot]);

  useEffect(() => {
    if (open) {
      loadInstances();
    }
  }, [open, loadInstances]);

  const handleCreateInstance = async () => {
    if (!newInstanceId) return;
    setLoading(true);
    try {
      const res = await fetch(`${apiRoot}/instances`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          instanceId: newInstanceId,
          instance: { 
            displayName: newInstanceName || newInstanceId,
            type: 'DEVELOPMENT'
          }
        })
      });
      if (res.ok) {
        showToast(`Instance ${newInstanceId} created`);
        loadInstances();
      } else {
        showToast('Failed to create instance', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    finally { setLoading(false); setNewInstanceOpen(false); setNewInstanceId(''); setNewInstanceName(''); }
  };

  const handleCreateTable = async () => {
    if (!newTableId || !targetInstance) return;
    setLoading(true);
    try {
      const res = await fetch(`${apiRoot}/instances/${targetInstance}/tables`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tableId: newTableId,
          table: { columnFamilies: {} }
        })
      });
      if (res.ok) {
        showToast(`Table ${newTableId} created in ${targetInstance}`);
        loadInstances();
      } else {
        showToast('Failed to create table', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    finally { setLoading(false); setNewTableOpen(false); setNewTableId(''); }
  };

  const handleDeleteInstance = async (id: string) => {
    if (!confirm('Are you sure you want to delete this instance?')) return;
    try {
      const res = await fetch(`${apiRoot}/instances/${id}`, { method: 'DELETE' });
      if (res.ok) {
        showToast(`Instance ${id} deleted`);
        loadInstances();
      }
    } catch (e: any) { showToast(e.message, 'error'); }
  };

  const handleDeleteTable = async (instId: string, tbId: string) => {
    try {
      const res = await fetch(`${apiRoot}/instances/${instId}/tables/${tbId}`, { method: 'DELETE' });
      if (res.ok) {
        showToast(`Table ${tbId} deleted`);
        loadInstances();
      }
    } catch (e: any) { showToast(e.message, 'error'); }
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: 500, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#e67c73' }}>Bigtable Console</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Manage Instances • {activeProject}</Typography>
            </Box>
            <Box>
              <IconButton onClick={() => loadInstances()} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Box sx={{ flex: 1, overflow: 'auto', p: 2 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Provisioned Instances</Typography>
              <Button size="small" variant="contained" startIcon={<AddIcon />} onClick={() => setNewInstanceOpen(true)}>Create Instance</Button>
            </Box>

            {instances.length === 0 && (
              <Paper elevation={0} sx={{ p: 4, textAlign: 'center', border: '1px dashed #dadce0', bgcolor: 'transparent' }}>
                <Typography variant="body2" sx={{ color: '#80868b' }}>No Bigtable instances found.</Typography>
              </Paper>
            )}

            <List sx={{ p: 0 }}>
              {instances.map(inst => {
                const instId = inst.name.split('/').pop();
                const tables = tablesByInstance[instId] || [];
                return (
                  <Paper key={instId} elevation={0} sx={{ mb: 2, border: '1px solid #dadce0', overflow: 'hidden' }}>
                    <Box sx={{ p: 1.5, bgcolor: '#f1f3f4', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <Box sx={{ display: 'flex', alignItems: 'center' }}>
                        <StorageIcon sx={{ fontSize: '1.2rem', mr: 1.5, color: '#5f6368' }} />
                        <Box>
                          <Typography sx={{ fontSize: '0.9rem', fontWeight: 600 }}>{instId}</Typography>
                          <Typography sx={{ fontSize: '0.75rem', color: '#5f6368' }}>{inst.displayName} • {inst.type}</Typography>
                        </Box>
                      </Box>
                      <Box>
                        <IconButton size="small" onClick={() => { setTargetInstance(instId); setNewTableOpen(true); }}><AddIcon fontSize="small" /></IconButton>
                        <IconButton size="small" onClick={() => handleDeleteInstance(instId)} color="error"><DeleteIcon fontSize="small" /></IconButton>
                      </Box>
                    </Box>
                    <Divider />
                    <List component="div" disablePadding sx={{ bgcolor: 'white' }}>
                      {tables.length === 0 && (
                        <Typography sx={{ fontSize: '0.75rem', p: 2, color: '#80868b', fontStyle: 'italic', textAlign: 'center' }}>No tables in this instance.</Typography>
                      )}
                      {tables.map(tb => {
                        const tbId = tb.name.split('/').pop();
                        return (
                          <ListItemButton key={tbId} sx={{ pl: 4, py: 0.5 }}>
                            <TableChartIcon sx={{ fontSize: '1rem', mr: 1.5, color: '#e67c73' }} />
                            <ListItemText primary={<Typography sx={{ fontSize: '0.8rem' }}>{tbId}</Typography>} />
                            <IconButton size="small" onClick={(e) => { e.stopPropagation(); handleDeleteTable(instId, tbId); }}><DeleteIcon fontSize="inherit" /></IconButton>
                          </ListItemButton>
                        );
                      })}
                    </List>
                  </Paper>
                );
              })}
            </List>
          </Box>
        </Box>
      </Drawer>

      <Dialog open={newInstanceOpen} onClose={() => setNewInstanceOpen(false)}>
        <DialogTitle>Create Bigtable Instance</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Instance ID" fullWidth variant="standard" sx={{ mb: 2 }}
                     value={newInstanceId} onChange={e => setNewInstanceId(e.target.value.replace(/[^a-z0-9-]/g, ''))} />
          <TextField margin="dense" label="Display Name" fullWidth variant="standard"
                     value={newInstanceName} onChange={e => setNewInstanceName(e.target.value)} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewInstanceOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateInstance} disabled={!newInstanceId} variant="contained">Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={newTableOpen} onClose={() => setNewTableOpen(false)}>
        <DialogTitle>Create Table in {targetInstance}</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Table ID" fullWidth variant="standard"
                     value={newTableId} onChange={e => setNewTableId(e.target.value.replace(/[^a-zA-Z0-9_.-]/g, ''))} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewTableOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateTable} disabled={!newTableId} variant="contained">Create</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
