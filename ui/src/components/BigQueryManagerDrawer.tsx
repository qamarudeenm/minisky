import { useState, useEffect, useCallback, useRef } from 'react';
import {
  Drawer, Box, Typography, IconButton, Paper, Button, TextField, Divider, List, ListItem, ListItemText, 
  ListItemIcon, CircularProgress, Snackbar, Tabs, Tab, Table, TableBody, TableCell, TableContainer, 
  TableHead, TableRow, Dialog, DialogTitle, DialogContent, DialogActions, Select, MenuItem, FormControl
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import RefreshIcon from '@mui/icons-material/Refresh';
import StorageIcon from '@mui/icons-material/Storage';
import TableChartIcon from '@mui/icons-material/TableChart';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import CloudUploadIcon from '@mui/icons-material/CloudUpload';
import LinkIcon from '@mui/icons-material/Link';
import FolderOpenIcon from '@mui/icons-material/FolderOpen';
import { useProjectContext } from '../contexts/ProjectContext';

type Props = { open: boolean; onClose: () => void };

type VisualField = { name: string; type: string; mode: string };

export default function BigQueryManagerDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();
  const [datasets, setDatasets] = useState<any[]>([]);
  const [tablesByDataset, setTablesByDataset] = useState<Record<string, any[]>>({});
  const [loading, setLoading] = useState(false);
  const [tabValue, setTabValue] = useState(0); // 0: Resources, 1: SQL Workspace, 2: Ingest Data
  const [toast, setToast] = useState({ open: false, msg: '', severity: 'success' as 'success' | 'error' });

  // SQL Workspace State
  const [sqlQuery, setSqlQuery] = useState('SELECT * FROM dataset1__table1 LIMIT 10;');
  const [queryResults, setQueryResults] = useState<any>(null);
  const [queryLoading, setQueryLoading] = useState(false);

  // Ingest State
  const [targetDataset, setTargetDataset] = useState('');
  const [targetTableId, setTargetTableId] = useState('');
  const [sourceUri, setSourceUri] = useState('');
  const [sourceFormat, setSourceFormat] = useState('CSV');
  const [ingesting, setIngesting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // New Dataset Dialog
  const [newDsOpen, setNewDsOpen] = useState(false);
  const [newDsId, setNewDsId] = useState('');

  // New Table Dialog (Visual Builder)
  const [newTbOpen, setNewTbOpen] = useState(false);
  const [targetDsForTb, setTargetDsForTb] = useState('');
  const [newTbId, setNewTbId] = useState('');
  const [visualFields, setVisualFields] = useState<VisualField[]>([
    { name: 'id', type: 'INTEGER', mode: 'REQUIRED' },
    { name: 'name', type: 'STRING', mode: 'NULLABLE' }
  ]);
  const [schemaTab, setSchemaTab] = useState(0); // 0: Visual, 1: JSON
  const [newTbSchemaJson, setNewTbSchemaJson] = useState('');

  const apiRoot = `/api/manage/bigquery/projects/${activeProject}`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadResources = useCallback(async () => {
    setLoading(true);
    try {
      const res = await fetch(`${apiRoot}/datasets`);
      if (res.ok) {
        const data = await res.json();
        const dsList = data.datasets || [];
        setDatasets(dsList);
        
        const tbResults: Record<string, any[]> = {};
        for (const ds of dsList) {
           const dsId = ds.datasetReference.datasetId;
           const tbRes = await fetch(`${apiRoot}/datasets/${dsId}/tables`);
           if (tbRes.ok) {
             const tbData = await tbRes.json();
             tbResults[dsId] = tbData.tables || [];
           }
        }
        setTablesByDataset(tbResults);
      }
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [apiRoot]);

  useEffect(() => {
    if (open) loadResources();
  }, [open, loadResources]);

  const executeSql = async () => {
    if (!sqlQuery) return;
    setQueryLoading(true);
    setQueryResults(null);
    try {
      const res = await fetch(`${apiRoot}/jobs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          configuration: {
            query: { query: sqlQuery, useLegacySql: false }
          }
        })
      });
      if (res.ok) {
        const job = await res.json();
        pollJobResults(job.jobReference.jobId);
      } else {
        showToast('Query failed', 'error');
        setQueryLoading(false);
      }
    } catch (e: any) { 
      showToast(e.message, 'error'); 
      setQueryLoading(false);
    }
  };

  const pollJobResults = async (jobId: string) => {
    const check = async () => {
      const res = await fetch(`${apiRoot}/jobs/${jobId}/results`);
      if (res.ok) {
        const data = await res.json();
        if (data.jobComplete) {
          setQueryResults(data);
          setQueryLoading(false);
          return true;
        }
      }
      return false;
    };

    const finished = await check();
    if (!finished) {
      const interval = setInterval(async () => {
        if (await check()) clearInterval(interval);
      }, 1000);
    }
  };

  const downloadResults = (format: 'CSV' | 'JSON') => {
    if (!queryResults || !queryResults.rows) return;
    
    let content = '';
    let fileName = `bigquery_results_${new Date().getTime()}`;
    const headers = queryResults.schema.fields.map((f: any) => f.name);

    if (format === 'CSV') {
      content = headers.join(',') + '\n';
      content += queryResults.rows.map((row: any) => 
        row.f.map((cell: any) => `"${cell.v}"`).join(',')
      ).join('\n');
      fileName += '.csv';
    } else {
      const jsonData = queryResults.rows.map((row: any) => {
        const item: any = {};
        row.f.forEach((cell: any, i: number) => {
          item[headers[i]] = cell.v;
        });
        return item;
      });
      content = JSON.stringify(jsonData, null, 2);
      fileName += '.json';
    }

    const blob = new Blob([content], { type: format === 'CSV' ? 'text/csv' : 'application/json' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = fileName;
    link.click();
    URL.revokeObjectURL(url);
  };

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setIngesting(true);
    const formData = new FormData();
    formData.append('file', file);

    try {
      const res = await fetch(`${apiRoot}/upload`, {
        method: 'POST',
        body: formData,
      });
      if (res.ok) {
        const data = await res.json();
        setSourceUri(data.path);
        if (!targetTableId) {
          setTargetTableId(data.filename.split('.')[0].replace(/[^a-zA-Z0-9_]/g, '_'));
        }
        showToast(`File uploaded: ${data.filename}`);
      } else {
        showToast('Upload failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    finally { setIngesting(false); }
  };

  const handleIngest = async () => {
    if (!targetDataset || !targetTableId || !sourceUri) return;
    setIngesting(true);
    try {
      const res = await fetch(`${apiRoot}/jobs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          configuration: {
            jobType: 'LOAD',
            load: {
              sourceUris: [sourceUri],
              destinationTable: {
                projectId: activeProject,
                datasetId: targetDataset,
                tableId: targetTableId
              },
              sourceFormat: sourceFormat,
              autodetect: true
            }
          }
        })
      });
      if (res.ok) {
        showToast(`Ingestion job started for ${targetTableId}`);
        setTimeout(loadResources, 2000);
        setTabValue(0);
      } else {
        showToast('Ingestion failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    finally { setIngesting(false); }
  };

  const createDataset = async () => {
    if (!newDsId) return;
    try {
      const res = await fetch(`${apiRoot}/datasets`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ datasetReference: { datasetId: newDsId } })
      });
      if (res.ok) {
        showToast(`Dataset ${newDsId} created`);
        loadResources();
        setNewDsOpen(false);
        setNewDsId('');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
  };

  const handleAddField = () => {
    setVisualFields([...visualFields, { name: '', type: 'STRING', mode: 'NULLABLE' }]);
  };

  const handleRemoveField = (index: number) => {
    setVisualFields(visualFields.filter((_, i) => i !== index));
  };

  const handleFieldChange = (index: number, key: keyof VisualField, value: string) => {
    const updated = [...visualFields];
    updated[index] = { ...updated[index], [key]: value };
    setVisualFields(updated);
  };

  const createTable = async () => {
    if (!newTbId || !targetDsForTb) return;
    try {
      let schema;
      if (schemaTab === 0) {
        schema = { fields: visualFields };
      } else {
        schema = JSON.parse(newTbSchemaJson);
      }

      const res = await fetch(`${apiRoot}/datasets/${targetDsForTb}/tables`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          tableReference: { tableId: newTbId },
          schema: schema
        })
      });
      if (res.ok) {
        showToast(`Table ${newTbId} created`);
        loadResources();
        setNewTbOpen(false);
        setNewTbId('');
      } else {
        showToast('Failed to create table', 'error');
      }
    } catch (e: any) { showToast('Invalid configuration', 'error'); }
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: 650, bgcolor: '#f8f9fa' }}>
        {/* Header */}
        <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 500, color: '#4285f4' }}>BigQuery Console</Typography>
            <Typography variant="caption" sx={{ color: '#5f6368' }}>{activeProject} • DuckDB Analytical Engine</Typography>
          </Box>
          <Box>
            <IconButton onClick={loadResources} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
            <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
          </Box>
        </Box>

        <Tabs value={tabValue} onChange={(_, v) => setTabValue(v)} sx={{ bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
          <Tab label="Resources" />
          <Tab label="SQL Workspace" />
          <Tab label="Ingest Data" />
        </Tabs>

        {tabValue === 0 && (
          <Box sx={{ p: 3, flexGrow: 1, overflowY: 'auto' }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Explorer</Typography>
              <Button variant="outlined" size="small" startIcon={<AddIcon />} onClick={() => setNewDsOpen(true)}>Create Dataset</Button>
            </Box>

            {loading && datasets.length === 0 ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}><CircularProgress size={24} /></Box>
            ) : datasets.length === 0 ? (
              <Paper elevation={0} sx={{ p: 4, textAlign: 'center', border: '1px dashed #dadce0', bgcolor: 'white' }}>
                <StorageIcon sx={{ fontSize: 48, color: '#dadce0', mb: 2 }} />
                <Typography variant="body1">No datasets found in {activeProject}.</Typography>
                <Button sx={{ mt: 2 }} variant="contained" onClick={() => setTabValue(2)}>Ingest your first CSV</Button>
              </Paper>
            ) : (
              <List>
                {datasets.map((ds) => {
                  const dsId = ds.datasetReference.datasetId;
                  const tbs = tablesByDataset[dsId] || [];
                  return (
                    <Paper key={dsId} sx={{ mb: 2, overflow: 'hidden', border: '1px solid #dadce0' }} elevation={0}>
                      <ListItem sx={{ bgcolor: '#fff' }}>
                        <ListItemIcon><StorageIcon color="primary" /></ListItemIcon>
                        <ListItemText 
                          primary={<Typography sx={{ fontWeight: 500 }}>{dsId}</Typography>}
                          secondary={ds.location} 
                        />
                        <Button size="small" startIcon={<AddIcon />} onClick={() => { setTargetDsForTb(dsId); setNewTbOpen(true); }}>Add Table</Button>
                      </ListItem>
                      <Divider />
                      <List disablePadding sx={{ bgcolor: '#fafafa' }}>
                        {tbs.length === 0 ? (
                          <ListItem><Typography variant="caption" sx={{ color: '#5f6368', pl: 7 }}>No tables</Typography></ListItem>
                        ) : tbs.map(tb => (
                          <ListItem key={tb.id} sx={{ pl: 7 }}>
                            <ListItemIcon sx={{ minWidth: 32 }}><TableChartIcon sx={{ fontSize: 18, color: '#5f6368' }} /></ListItemIcon>
                            <ListItemText primary={<Typography variant="body2">{tb.tableReference.tableId}</Typography>} />
                            <IconButton size="small" onClick={() => {
                              setSqlQuery(`SELECT * FROM ${dsId}__${tb.tableReference.tableId} LIMIT 10;`);
                              setTabValue(1);
                            }}><PlayArrowIcon sx={{ fontSize: 16 }} /></IconButton>
                          </ListItem>
                        ))}
                      </List>
                    </Paper>
                  );
                })}
              </List>
            )}
          </Box>
        )}

        {tabValue === 1 && (
          <Box sx={{ p: 3, flexGrow: 1, display: 'flex', flexDirection: 'column' }}>
            <Box sx={{ mb: 2, display: 'flex', gap: 2 }}>
              <TextField
                multiline
                rows={6}
                fullWidth
                variant="outlined"
                placeholder="Enter Standard SQL..."
                value={sqlQuery}
                onChange={(e) => setSqlQuery(e.target.value)}
                sx={{ bgcolor: 'white' }}
                slotProps={{ input: { sx: { fontFamily: 'monospace', fontSize: '0.9rem' } } }}
              />
              <Button 
                variant="contained" 
                sx={{ height: 'fit-content' }}
                startIcon={queryLoading ? <CircularProgress size={16} color="inherit" /> : <PlayArrowIcon />}
                onClick={executeSql}
                disabled={queryLoading}
              >
                Run
              </Button>
            </Box>

            <Box sx={{ flexGrow: 1, overflow: 'auto' }}>
              {queryResults ? (
                <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                  <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 1, gap: 1 }}>
                    <Button size="small" variant="outlined" onClick={() => downloadResults('CSV')}>Export CSV</Button>
                    <Button size="small" variant="outlined" onClick={() => downloadResults('JSON')}>Export JSON</Button>
                  </Box>
                  <TableContainer component={Paper} elevation={0} sx={{ border: '1px solid #dadce0', flexGrow: 1 }}>
                    <Table size="small" stickyHeader>
                      <TableHead>
                        <TableRow>
                          {queryResults.schema?.fields?.map((f: any) => (
                            <TableCell key={f.name} sx={{ bgcolor: '#f8f9fa', fontWeight: 600 }}>{f.name}</TableCell>
                          ))}
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {queryResults.rows?.map((row: any, i: number) => (
                          <TableRow key={i}>
                            {row.f.map((cell: any, j: number) => (
                              <TableCell key={j}>{cell.v}</TableCell>
                            ))}
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                    {(!queryResults.rows || queryResults.rows.length === 0) && (
                      <Box sx={{ p: 4, textAlign: 'center', color: '#5f6368' }}>Query returned 0 rows.</Box>
                    )}
                  </TableContainer>
                </Box>
              ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#dadce0' }}>
                  <PlayArrowIcon sx={{ fontSize: 64, mb: 1 }} />
                  <Typography>Run a query to see results</Typography>
                </Box>
              )}
            </Box>
          </Box>
        )}

        {tabValue === 2 && (
          <Box sx={{ p: 3, flexGrow: 1 }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 3 }}>Ingest Data (CSV / JSON / Sheets)</Typography>
            
            <Paper sx={{ p: 3, border: '1px solid #dadce0' }} elevation={0}>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                <Box sx={{ display: 'flex', gap: 2 }}>
                  <TextField
                    select
                    label="Target Dataset"
                    fullWidth
                    value={targetDataset}
                    onChange={(e) => setTargetDataset(e.target.value)}
                    slotProps={{ select: { native: true } }}
                  >
                    <option value="">Select Dataset...</option>
                    {datasets.map(ds => (
                      <option key={ds.id} value={ds.datasetReference.datasetId}>{ds.datasetReference.datasetId}</option>
                    ))}
                  </TextField>
                  <TextField
                    label="Target Table Name"
                    fullWidth
                    value={targetTableId}
                    onChange={(e) => setTargetTableId(e.target.value)}
                    placeholder="e.g. users_from_sheet"
                  />
                </Box>

                <Box sx={{ display: 'flex', gap: 1 }}>
                  <TextField
                    label="Source URI (URL or Local Path)"
                    fullWidth
                    value={sourceUri}
                    onChange={(e) => setSourceUri(e.target.value)}
                    placeholder="e.g. https://docs.google.com/spreadsheets/d/.../export?format=csv"
                    helperText="Tip: For Google Sheets, use the CSV export link. For local files, click Browse."
                  />
                  <Button 
                    variant="outlined" 
                    startIcon={<FolderOpenIcon />}
                    sx={{ height: 56 }}
                    onClick={() => fileInputRef.current?.click()}
                  >
                    Browse
                  </Button>
                  <input 
                    type="file" 
                    ref={fileInputRef} 
                    style={{ display: 'none' }} 
                    accept=".csv,.json,.jsonl,.parquet"
                    onChange={handleFileUpload}
                  />
                </Box>

                <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
                  <TextField
                    select
                    label="Source Format"
                    sx={{ width: 200 }}
                    value={sourceFormat}
                    onChange={(e) => setSourceFormat(e.target.value)}
                    slotProps={{ select: { native: true } }}
                  >
                    <option value="CSV">CSV (Auto-detect)</option>
                    <option value="JSON">JSON (Newline Delimited)</option>
                    <option value="PARQUET">Parquet</option>
                  </TextField>
                  <Button 
                    variant="contained" 
                    startIcon={ingesting ? <CircularProgress size={16} color="inherit" /> : <CloudUploadIcon />}
                    size="large"
                    onClick={handleIngest}
                    disabled={!targetDataset || !targetTableId || !sourceUri || ingesting}
                  >
                    Start Ingestion
                  </Button>
                </Box>
              </Box>
            </Paper>

            <Box sx={{ mt: 4, p: 2, bgcolor: '#e8f0fe', borderRadius: 1, display: 'flex', gap: 2 }}>
              <LinkIcon sx={{ color: '#1967d2' }} />
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 600, color: '#1967d2' }}>Google Sheets Integration</Typography>
                <Typography variant="caption" sx={{ color: '#1967d2' }}>
                  To connect a Google Sheet, click File &gt; Share &gt; Publish to web. Choose "Comma-separated values (.csv)" 
                  and copy the link into the Source URI field above.
                </Typography>
              </Box>
            </Box>
          </Box>
        )}
      </Box>

      {/* Dialogs */}
      <Dialog open={newDsOpen} onClose={() => setNewDsOpen(false)}>
        <DialogTitle>Create BigQuery Dataset</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Dataset ID" fullWidth variant="outlined" value={newDsId} onChange={(e) => setNewDsId(e.target.value)} placeholder="e.g. my_analytics" sx={{ mt: 1 }} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewDsOpen(false)}>Cancel</Button>
          <Button onClick={createDataset} variant="contained" disabled={!newDsId}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={newTbOpen} onClose={() => setNewTbOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>Create Table in {targetDsForTb}</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Table ID" fullWidth variant="outlined" value={newTbId} onChange={(e) => setNewTbId(e.target.value)} placeholder="e.g. users" sx={{ mt: 1, mb: 3 }} />
          
          <Tabs value={schemaTab} onChange={(_, v) => setSchemaTab(v)} sx={{ mb: 2 }}>
            <Tab label="Visual Builder" />
            <Tab label="JSON Schema" />
          </Tabs>

          {schemaTab === 0 ? (
            <Box>
              <Box sx={{ display: 'flex', bgcolor: '#f8f9fa', p: 1, borderRadius: 1, mb: 1 }}>
                <Typography sx={{ flex: 2, fontSize: '0.8rem', fontWeight: 600 }}>Field Name</Typography>
                <Typography sx={{ flex: 1.5, fontSize: '0.8rem', fontWeight: 600 }}>Type</Typography>
                <Typography sx={{ flex: 1.5, fontSize: '0.8rem', fontWeight: 600 }}>Mode</Typography>
                <Box sx={{ width: 40 }} />
              </Box>
              <Box sx={{ maxHeight: 300, overflowY: 'auto' }}>
                {visualFields.map((field, i) => (
                  <Box key={i} sx={{ display: 'flex', gap: 1, mb: 1, alignItems: 'center' }}>
                    <TextField size="small" sx={{ flex: 2 }} value={field.name} onChange={(e) => handleFieldChange(i, 'name', e.target.value)} placeholder="column_name" />
                    <FormControl size="small" sx={{ flex: 1.5 }}>
                      <Select value={field.type} onChange={(e) => handleFieldChange(i, 'type', e.target.value)}>
                        <MenuItem value="STRING">STRING</MenuItem>
                        <MenuItem value="INTEGER">INTEGER</MenuItem>
                        <MenuItem value="FLOAT">FLOAT</MenuItem>
                        <MenuItem value="BOOLEAN">BOOLEAN</MenuItem>
                        <MenuItem value="TIMESTAMP">TIMESTAMP</MenuItem>
                        <MenuItem value="DATE">DATE</MenuItem>
                        <MenuItem value="JSON">JSON</MenuItem>
                      </Select>
                    </FormControl>
                    <FormControl size="small" sx={{ flex: 1.5 }}>
                      <Select value={field.mode} onChange={(e) => handleFieldChange(i, 'mode', e.target.value)}>
                        <MenuItem value="NULLABLE">NULLABLE</MenuItem>
                        <MenuItem value="REQUIRED">REQUIRED</MenuItem>
                      </Select>
                    </FormControl>
                    <IconButton size="small" color="error" onClick={() => handleRemoveField(i)}><DeleteIcon fontSize="small" /></IconButton>
                  </Box>
                ))}
              </Box>
              <Button startIcon={<AddIcon />} size="small" onClick={handleAddField} sx={{ mt: 1 }}>Add Field</Button>
            </Box>
          ) : (
            <TextField multiline rows={10} label="Table Schema (JSON)" fullWidth variant="outlined" value={newTbSchemaJson} onChange={(e) => setNewTbSchemaJson(e.target.value)} slotProps={{ input: { sx: { fontFamily: 'monospace', fontSize: '0.8rem' } } }} placeholder='{"fields": [...]}' />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewTbOpen(false)}>Cancel</Button>
          <Button onClick={createTable} variant="contained" disabled={!newTbId}>Create</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={4000} onClose={() => setToast({ ...toast, open: false })} message={toast.msg} />
    </Drawer>
  );
}
