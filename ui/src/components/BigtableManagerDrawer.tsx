import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItemButton, ListItemText, TextField,
  Snackbar, Alert, Paper, Dialog,
  DialogTitle, DialogContent, DialogActions,
  Divider, Tabs, Tab, Table, TableBody, TableCell, TableContainer, TableHead, TableRow,
  CircularProgress
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import RefreshIcon from '@mui/icons-material/Refresh';
import StorageIcon from '@mui/icons-material/Storage';
import TableChartIcon from '@mui/icons-material/TableChart';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import SearchIcon from '@mui/icons-material/Search';
import { useProjectContext } from '../contexts/ProjectContext';

type BigtableManagerDrawerProps = { open: boolean; onClose: () => void };

export default function BigtableManagerDrawer({ open, onClose }: BigtableManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  
  const [instances, setInstances] = useState<any[]>([]);
  const [tablesByInstance, setTablesByInstance] = useState<Record<string, any[]>>({});
  
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });
  const [tabValue, setTabValue] = useState(0);

  // Data Explorer State
  const [selectedTablePath, setSelectedTablePath] = useState(''); // instance/table
  const [rows, setRows] = useState<any[]>([]);
  const [dataLoading, setDataLoading] = useState(false);

  // Creation State
  const [newInstanceOpen, setNewInstanceOpen] = useState(false);
  const [newInstanceId, setNewInstanceId] = useState('');
  const [newInstanceName, setNewInstanceName] = useState('');
  
  const [newTableOpen, setNewTableOpen] = useState(false);
  const [targetInstance, setTargetInstance] = useState('');
  const [newTableId, setNewTableId] = useState('');

  const apiRoot = `/api/manage/bigtable/projects/${activeProject}`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadInstances = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${apiRoot}/instances`);
      if (res.ok) {
        const data = await res.json();
        const instList = data.instances || [];
        setInstances(instList);
        
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
    finally { setLoading(false); }
  }, [apiRoot]);

  useEffect(() => {
    if (open) {
      loadInstances();
    }
  }, [open, loadInstances]);

  const loadRows = async () => {
    if (!selectedTablePath) return;
    setDataLoading(true);
    try {
      const [instId, tbId] = selectedTablePath.split('/');
      const res = await fetch(`${apiRoot}/instances/${instId}/tables/${tbId}:readRows`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({})
      });
      if (res.ok) {
        const data = await res.json();
        setRows(data.rows || []);
      } else {
        showToast('Failed to load rows', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    finally { setDataLoading(false); }
  };

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
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: 600, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#e67c73' }}>Bigtable Console</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>{activeProject} • Bigtable Emulator</Typography>
            </Box>
            <Box>
              <IconButton onClick={() => loadInstances()} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Tabs value={tabValue} onChange={(_, v) => setTabValue(v)} sx={{ bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Tab label="Resources" />
            <Tab label="Data Explorer" />
          </Tabs>

          {tabValue === 0 && (
            <Box sx={{ flex: 1, overflow: 'auto', p: 3 }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Provisioned Instances</Typography>
                <Button size="small" variant="contained" startIcon={<AddIcon />} onClick={() => setNewInstanceOpen(true)}>Create Instance</Button>
              </Box>

              {loading && instances.length === 0 ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}><CircularProgress size={24} /></Box>
              ) : instances.length === 0 ? (
                <Paper elevation={0} sx={{ p: 4, textAlign: 'center', border: '1px dashed #dadce0', bgcolor: 'transparent' }}>
                  <Typography variant="body2" sx={{ color: '#80868b' }}>No Bigtable instances found.</Typography>
                </Paper>
              ) : (
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
                            const path = `${instId}/${tbId}`;
                            return (
                              <ListItemButton 
                                key={tbId} 
                                sx={{ pl: 4, py: 0.5 }}
                                onClick={() => {
                                  setSelectedTablePath(path);
                                  setTabValue(1);
                                  setTimeout(loadRows, 100);
                                }}
                              >
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
              )}
            </Box>
          )}

          {tabValue === 1 && (
            <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', p: 3 }}>
              <Box sx={{ display: 'flex', gap: 2, mb: 2 }}>
                <TextField
                  select
                  label="Select Table"
                  value={selectedTablePath}
                  onChange={(e) => setSelectedTablePath(e.target.value)}
                  slotProps={{ select: { native: true } }}
                  size="small"
                  sx={{ flex: 1 }}
                >
                  <option value="">Choose a table...</option>
                  {Object.entries(tablesByInstance).flatMap(([instId, tables]) => 
                    tables.map(tb => {
                      const tbId = tb.name.split('/').pop();
                      return <option key={`${instId}/${tbId}`} value={`${instId}/${tbId}`}>{instId} / {tbId}</option>
                    })
                  )}
                </TextField>
                <Button variant="contained" startIcon={<SearchIcon />} onClick={loadRows} disabled={!selectedTablePath || dataLoading}>
                  Scan
                </Button>
              </Box>

              <Box sx={{ flex: 1, overflow: 'auto' }}>
                {dataLoading ? (
                  <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}><CircularProgress size={32} /></Box>
                ) : rows.length === 0 ? (
                  <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#dadce0' }}>
                    <SearchIcon sx={{ fontSize: 64, mb: 1 }} />
                    <Typography>No data found or table empty</Typography>
                  </Box>
                ) : (
                  <TableContainer component={Paper} elevation={0} sx={{ border: '1px solid #dadce0' }}>
                    <Table size="small" stickyHeader>
                      <TableHead>
                        <TableRow>
                          <TableCell sx={{ bgcolor: '#f8f9fa', fontWeight: 600 }}>Row Key</TableCell>
                          <TableCell sx={{ bgcolor: '#f8f9fa', fontWeight: 600 }}>Columns (Family:Qualifier → Value)</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {rows.map((row) => (
                          <TableRow key={row.key}>
                            <TableCell sx={{ fontFamily: 'monospace', verticalAlign: 'top' }}>{row.key}</TableCell>
                            <TableCell>
                              {Object.entries(row.data).map(([family, columns]: [string, any]) => (
                                <Box key={family} sx={{ mb: 1 }}>
                                  <Typography variant="caption" sx={{ fontWeight: 600, color: '#e67c73' }}>{family}:</Typography>
                                  {Object.entries(columns).map(([col, val]: [string, any]) => (
                                    <Box key={col} sx={{ pl: 1, display: 'flex', gap: 1 }}>
                                      <Typography variant="caption" sx={{ color: '#5f6368' }}>{col.split(':').pop()} →</Typography>
                                      <Typography variant="caption" sx={{ fontWeight: 500 }}>{val}</Typography>
                                    </Box>
                                  ))}
                                </Box>
                              ))}
                            </TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </TableContainer>
                )}
              </Box>
            </Box>
          )}
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
