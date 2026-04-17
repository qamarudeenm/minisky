import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  List, ListItemButton, ListItemText, TextField,
  Snackbar, Alert, CircularProgress, Paper, Dialog,
  DialogTitle, DialogContent, DialogActions,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RefreshIcon from '@mui/icons-material/Refresh';
import StorageIcon from '@mui/icons-material/Storage';
import TableChartIcon from '@mui/icons-material/TableChart';
import AddIcon from '@mui/icons-material/Add';
import { useProjectContext } from '../contexts/ProjectContext';

type BigQueryManagerDrawerProps = { open: boolean; onClose: () => void };

export default function BigQueryManagerDrawer({ open, onClose }: BigQueryManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  
  const [datasets, setDatasets] = useState<any[]>([]);
  const [tablesByDataset, setTablesByDataset] = useState<Record<string, any[]>>({});
  
  const [loading, setLoading] = useState(false);
  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });

  // SQL State
  const [sqlQuery, setSqlQuery] = useState('-- Write Standard SQL to execute against the DuckDB backend\nSELECT * FROM test_dataset.my_table limit 10;');
  const [queryResults, setQueryResults] = useState<{ schema: any, rows: any[] } | null>(null);

  // Creation State
  const [newDatasetOpen, setNewDatasetOpen] = useState(false);
  const [newDatasetId, setNewDatasetId] = useState('');
  
  const [newTableOpen, setNewTableOpen] = useState(false);
  const [targetDataset, setTargetDataset] = useState('');
  const [newTableId, setNewTableId] = useState('');
  const [newTableFields, setNewTableFields] = useState('-- Define fields: name, type, mode\n[\n  {"name": "id", "type": "INTEGER", "mode": "REQUIRED"},\n  {"name": "name", "type": "STRING", "mode": "NULLABLE"}\n]');

  const apiRoot = `/api/manage/bigquery/projects/${activeProject}`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadDatasets = useCallback(async () => {
    try {
      const res = await fetch(`${apiRoot}/datasets`);
      if (res.ok) {
        const data = await res.json();
        const dsList = data.datasets || [];
        setDatasets(dsList);
        
        // Load tables for each dataset
        const tbMap: Record<string, any[]> = {};
        for (const ds of dsList) {
          const dsId = ds.datasetReference.datasetId;
          const tbRes = await fetch(`${apiRoot}/datasets/${dsId}/tables`);
          if (tbRes.ok) {
            const tbData = await tbRes.json();
            tbMap[dsId] = tbData.tables || [];
          }
        }
        setTablesByDataset(tbMap);
      }
    } catch (e) { console.error(e); }
  }, [apiRoot]);

  useEffect(() => {
    if (open) {
      loadDatasets();
    }
  }, [open, loadDatasets]);

  const handleRunQuery = async () => {
    if (!sqlQuery.trim()) return;
    setLoading(true);
    setQueryResults(null);
    try {
      // 1. Submit Job
      const jobRes = await fetch(`${apiRoot}/jobs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          configuration: {
            query: { query: sqlQuery }
          }
        })
      });
      if (!jobRes.ok) {
        throw new Error('Failed to submit job');
      }
      const jobData = await jobRes.json();
      const jobId = jobData.jobReference.jobId;

      // 2. Poll for results
      let attempts = 0;
      let done = false;
      while (!done && attempts < 10) {
        await new Promise(r => setTimeout(r, 1000)); // Poll every 1s
        attempts++;
        const resRes = await fetch(`${apiRoot}/jobs/${jobId}/results`);
        if (resRes.ok) {
          const resData = await resRes.json();
          if (resData.jobComplete) {
            done = true;
            if (resData.errors && resData.errors.length > 0) {
              throw new Error(resData.errors[0].message);
            }
            setQueryResults({
              schema: resData.schema,
              rows: resData.rows
            });
            showToast('Query completed successfully');
          }
        }
      }
      if (!done) showToast('Query timeout', 'error');
    } catch (e: any) {
      showToast('Execution error: ' + e.message, 'error');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateDataset = async () => {
    if (!newDatasetId) return;
    try {
      const res = await fetch(`${apiRoot}/datasets`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ datasetReference: { datasetId: newDatasetId } })
      });
      if (res.ok) {
        showToast(`Dataset ${newDatasetId} created`);
        loadDatasets();
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed to create dataset', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewDatasetOpen(false);
    setNewDatasetId('');
  };

  const handleCreateTable = async () => {
    if (!newTableId || !targetDataset) return;
    try {
      let schemaFields = [];
      try {
        schemaFields = JSON.parse(newTableFields);
      } catch (e) {
        showToast('Invalid JSON schema format', 'error');
        return;
      }

      const res = await fetch(`${apiRoot}/datasets/${targetDataset}/tables`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tableReference: { tableId: newTableId },
          schema: { fields: schemaFields }
        })
      });
      if (res.ok) {
        showToast(`Table ${newTableId} created in ${targetDataset}`);
        loadDatasets();
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed to create table', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewTableOpen(false);
    setNewTableId('');
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '85vw', maxWidth: 1400, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#1a73e8' }}>BigQuery SQL Workspace</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Powered by embedded DuckDB • {activeProject}</Typography>
            </Box>
            <Box>
              <IconButton onClick={() => loadDatasets()} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Box sx={{ display: 'flex', flex: 1, overflow: 'hidden' }}>
            {/* Left Pane - Explorer */}
            <Box sx={{ width: 280, borderRight: '1px solid #dadce0', bgcolor: 'white', display: 'flex', flexDirection: 'column' }}>
              <Box sx={{ p: 1.5, borderBottom: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Explorer</Typography>
                <IconButton size="small" onClick={() => setNewDatasetOpen(true)}><AddIcon fontSize="small" /></IconButton>
              </Box>
              <List sx={{ flex: 1, overflow: 'auto', p: 0 }}>
                {datasets.length === 0 && (
                  <Typography variant="body2" sx={{ p: 2, color: '#80868b', fontStyle: 'italic' }}>No datasets found.</Typography>
                )}
                {datasets.map(ds => {
                  const dsId = ds.datasetReference.datasetId;
                  const tables = tablesByDataset[dsId] || [];
                  return (
                    <Box key={dsId}>
                      <ListItemButton sx={{ py: 0.5, bgcolor: '#f1f3f4', display: 'flex', justifyContent: 'space-between' }}>
                        <Box sx={{ display: 'flex', alignItems: 'center' }}>
                          <StorageIcon sx={{ fontSize: '1.1rem', mr: 1.5, color: '#5f6368' }} />
                          <ListItemText primary={<Typography sx={{ fontSize: '0.85rem', fontWeight: 600 }}>{dsId}</Typography>} />
                        </Box>
                        <IconButton size="small" onClick={(e) => { e.stopPropagation(); setTargetDataset(dsId); setNewTableOpen(true); }}>
                          <AddIcon fontSize="inherit" />
                        </IconButton>
                      </ListItemButton>
                      <List component="div" disablePadding>
                        {tables.length === 0 && (
                          <ListItemText 
                            primary={<Typography sx={{ fontSize: '0.75rem', pl: 5, pb: 1, color: '#80868b', fontStyle: 'italic' }}>No tables</Typography>} 
                          />
                        )}
                        {tables.map(tb => {
                          const tbId = tb.tableReference.tableId;
                          return (
                            <ListItemButton key={tbId} sx={{ pl: 5, py: 0.5 }}>
                              <TableChartIcon sx={{ fontSize: '1rem', mr: 1.5, color: '#1a73e8' }} />
                              <ListItemText primary={<Typography sx={{ fontSize: '0.8rem' }}>{tbId}</Typography>} />
                            </ListItemButton>
                          );
                        })}
                      </List>
                    </Box>
                  );
                })}
              </List>
            </Box>

            {/* Right Pane - Query Editor & Results */}
            <Box sx={{ flex: 1, p: 2, display: 'flex', flexDirection: 'column', bgcolor: '#f8f9fa', gap: 2, overflow: 'hidden' }}>
              
              {/* Editor */}
              <Box sx={{ display: 'flex', flexDirection: 'column', height: '40%' }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Query Editor</Typography>
                  <Button variant="contained" size="small" startIcon={loading ? <CircularProgress size={16} color="inherit" /> : <PlayArrowIcon />} onClick={handleRunQuery} disabled={loading}>
                    Run Query
                  </Button>
                </Box>
                <Paper elevation={0} sx={{ flex: 1, display: 'flex' }}>
                  <TextField
                    multiline
                    fullWidth
                    variant="outlined"
                    value={sqlQuery}
                    onChange={e => setSqlQuery(e.target.value)}
                    sx={{ height: '100%', '& .MuiInputBase-root': { height: '100%', alignItems: 'flex-start', fontFamily: 'monospace', fontSize: '0.85rem', bgcolor: '#fff' } }}
                  />
                </Paper>
              </Box>

              {/* Results */}
              <Box sx={{ display: 'flex', flexDirection: 'column', height: '60%' }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>Query Results</Typography>
                </Box>
                <Paper elevation={0} sx={{ flex: 1, overflow: 'auto', bgcolor: '#fff', border: '1px solid #dadce0' }}>
                  {!queryResults ? (
                    <Box sx={{ p: 4, textAlign: 'center', color: '#80868b' }}>
                      <Typography variant="body2">No results to display. Run a query above.</Typography>
                    </Box>
                  ) : (
                    <TableContainer>
                      <Table size="small" stickyHeader>
                        <TableHead>
                          <TableRow>
                            {queryResults.schema?.fields?.map((f: any, i: number) => (
                              <TableCell key={i} sx={{ fontWeight: 600, bgcolor: '#f1f3f4', fontSize: '0.8rem' }}>{f.name}</TableCell>
                            ))}
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {queryResults.rows?.map((row, i) => (
                            <TableRow key={i}>
                              {row.f?.map((cell: any, j: number) => (
                                <TableCell key={j} sx={{ fontSize: '0.8rem', fontFamily: 'monospace' }}>{cell.v}</TableCell>
                              ))}
                            </TableRow>
                          ))}
                          {(!queryResults.rows || queryResults.rows.length === 0) && (
                            <TableRow>
                              <TableCell colSpan={queryResults.schema?.fields?.length || 1} sx={{ textAlign: 'center', py: 3, fontStyle: 'italic', color: '#80868b' }}>
                                Query returned zero rows.
                              </TableCell>
                            </TableRow>
                          )}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  )}
                </Paper>
              </Box>

            </Box>
          </Box>
        </Box>
      </Drawer>

      <Dialog open={newDatasetOpen} onClose={() => setNewDatasetOpen(false)}>
        <DialogTitle>Create Dataset</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Dataset ID" fullWidth variant="standard"
                     value={newDatasetId} onChange={e => setNewDatasetId(e.target.value.replace(/[^a-zA-Z0-9_]/g, ''))} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewDatasetOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateDataset} disabled={!newDatasetId}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={newTableOpen} onClose={() => setNewTableOpen(false)}>
        <DialogTitle>Create Table in {targetDataset}</DialogTitle>
        <DialogContent sx={{ minWidth: 400 }}>
          <TextField autoFocus margin="dense" label="Table ID" fullWidth variant="standard" sx={{ mb: 2 }}
                     value={newTableId} onChange={e => setNewTableId(e.target.value.replace(/[^a-zA-Z0-9_]/g, ''))} />
          <Typography variant="caption" sx={{ color: '#5f6368', mb: 1, display: 'block' }}>Schema (JSON Array)</Typography>
          <TextField
              multiline
              fullWidth
              variant="outlined"
              rows={6}
              value={newTableFields}
              onChange={e => setNewTableFields(e.target.value)}
              sx={{ '& .MuiInputBase-root': { fontFamily: 'monospace', fontSize: '0.8rem' } }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewTableOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateTable} disabled={!newTableId}>Create</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
