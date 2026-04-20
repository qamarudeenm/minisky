import {
  Drawer, Box, Typography, IconButton, Paper, Button, TextField, Divider, List, ListItem, ListItemText, 
  ListItemIcon, CircularProgress, Snackbar, Dialog, DialogTitle, DialogContent, DialogActions,
  Tabs, Tab, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Collapse, ListItemButton
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import StorageIcon from '@mui/icons-material/Storage';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import TableRowsIcon from '@mui/icons-material/TableRows';
import TerminalIcon from '@mui/icons-material/Terminal';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import SecurityIcon from '@mui/icons-material/Security';
import { useProjectContext } from '../contexts/ProjectContext';
import { useState, useEffect, useCallback } from 'react';

type Props = { open: boolean; onClose: () => void };

export default function SpannerManagerDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();
  const [instances, setInstances] = useState<any[]>([]);
  const [databasesByInstance, setDatabasesByInstance] = useState<Record<string, any[]>>({});
  const [tablesByDb, setTablesByDb] = useState<Record<string, any[]>>({});
  const [loading, setLoading] = useState(false);
  const [tabValue, setTabValue] = useState(0); // 0: Instances, 1: SQL Workspace
  const [toast, setToast] = useState({ open: false, msg: '', severity: 'success' as 'success' | 'error' });

  // SQL Workspace State
  const [selectedDb, setSelectedDb] = useState('');
  const [sqlQuery, setSqlQuery] = useState('SELECT * FROM INFORMATION_SCHEMA.TABLES;');
  const [queryResults, setQueryResults] = useState<any>(null);
  const [queryLoading, setQueryLoading] = useState(false);

  // Dialog State
  const [newInstanceOpen, setNewInstanceOpen] = useState(false);
  const [newInstanceId, setNewInstanceId] = useState('');
  const [newDbOpen, setNewDbOpen] = useState(false);
  const [targetInstance, setTargetInstance] = useState('');
  const [newDbId, setNewDbId] = useState('');
  const [expandedDb, setExpandedDb] = useState<string | null>(null);

  const apiRoot = `/api/manage/spanner/projects/${activeProject}`;

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
        
        const dbMap: Record<string, any[]> = {};
        for (const inst of instList) {
          const instId = inst.name.split('/').pop();
          const dbRes = await fetch(`${apiRoot}/instances/${instId}/databases`);
          if (dbRes.ok) {
            const dbData = await dbRes.json();
            dbMap[instId] = dbData.databases || [];
          }
        }
        setDatabasesByInstance(dbMap);
      }
    } catch (e) {
      console.error("Failed to load Spanner resources", e);
    } finally {
      setLoading(false);
    }
  }, [apiRoot]);

  const createSession = async (instId: string, dbId: string) => {
    const res = await fetch(`${apiRoot}/instances/${instId}/databases/${dbId}/sessions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({})
    });
    if (!res.ok) throw new Error('Failed to create session');
    const data = await res.json();
    return data.name; // full resource name
  };

  const loadTables = async (instId: string, dbId: string) => {
    const dbPath = `${instId}/${dbId}`;
    try {
      // 1. Create a session
      const sessionName = await createSession(instId, dbId);
      const sessionPath = sessionName.split('/').pop();

      // 2. Execute SQL
      const res = await fetch(`${apiRoot}/instances/${instId}/databases/${dbId}/sessions/${sessionPath}:executeSql`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          sql: "SELECT table_name FROM information_schema.tables WHERE table_schema = ''" 
        })
      });
      if (res.ok) {
        const data = await res.json();
        const tables = data.rows?.map((r: any) => r[0]) || [];
        setTablesByDb(prev => ({ ...prev, [dbPath]: tables }));
      }
    } catch (e) {
      console.error("Failed to load tables", e);
    }
  };

  const executeSql = async () => {
    if (!selectedDb || !sqlQuery) return;
    setQueryLoading(true);
    setQueryResults(null);
    try {
      const [instId, dbId] = selectedDb.split('/');
      const isDDL = /^\s*(CREATE|ALTER|DROP)\s+/i.test(sqlQuery);
      
      let url = '';
      let body: any = {};
      let method = 'POST';

      if (isDDL) {
        url = `${apiRoot}/instances/${instId}/databases/${dbId}/ddl`;
        body = { statements: [sqlQuery] };
        method = 'PATCH';
      } else {
        // For queries, we need a session
        const sessionName = await createSession(instId, dbId);
        const sessionPath = sessionName.split('/').pop();
        url = `${apiRoot}/instances/${instId}/databases/${dbId}/sessions/${sessionPath}:executeSql`;
        body = { sql: sqlQuery };
        method = 'POST';
      }

      const res = await fetch(url, {
        method: method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      
      const data = await res.json();
      if (res.ok) {
        setQueryResults(isDDL ? { message: 'Schema updated successfully' } : data);
        showToast(isDDL ? 'Schema updated' : 'Query executed successfully');
        if (isDDL) loadTables(instId, dbId);
      } else {
        const errMsg = data.error?.message || data.message || 'Operation failed';
        showToast(errMsg, 'error');
      }
    } catch (e: any) {
      showToast(e.message, 'error');
    } finally {
      setQueryLoading(false);
    }
  };

  useEffect(() => {
    if (open) loadInstances();
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
            displayName: newInstanceId,
            config: `projects/${activeProject}/instanceConfigs/emulator-config`,
            nodeCount: 1
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
    finally { setLoading(false); setNewInstanceOpen(false); setNewInstanceId(''); }
  };

  const handleCreateDatabase = async () => {
    if (!newDbId || !targetInstance) return;
    setLoading(true);
    try {
      const res = await fetch(`${apiRoot}/instances/${targetInstance}/databases`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ createStatement: `CREATE DATABASE \`${newDbId}\`` })
      });
      if (res.ok) {
        showToast(`Database ${newDbId} created`);
        loadInstances();
      } else {
        showToast('Failed to create database', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    finally { setLoading(false); setNewDbOpen(false); setNewDbId(''); }
  };

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: 650, bgcolor: '#f8f9fa' }}>
        {/* Header */}
        <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 500, color: '#1a73e8' }}>Cloud Spanner Console</Typography>
            <Typography variant="caption" sx={{ color: '#5f6368' }}>{activeProject} • Spanner Emulator</Typography>
          </Box>
          <Box>
            <IconButton onClick={loadInstances} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
            <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
          </Box>
        </Box>

        <Tabs value={tabValue} onChange={(_, v) => setTabValue(v)} sx={{ bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
          <Tab label="Resources" />
          <Tab label="SQL Workspace" />
        </Tabs>

        {tabValue === 0 && (
          <Box sx={{ p: 3, flexGrow: 1, overflowY: 'auto' }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>Topology</Typography>
              <Button variant="contained" size="small" startIcon={<AddIcon />} onClick={() => setNewInstanceOpen(true)}>New Instance</Button>
            </Box>

            {loading && instances.length === 0 ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}><CircularProgress size={24} /></Box>
            ) : instances.length === 0 ? (
              <Paper elevation={0} sx={{ p: 4, textAlign: 'center', border: '1px dashed #dadce0', bgcolor: 'white' }}>
                <StorageIcon sx={{ fontSize: 48, color: '#dadce0', mb: 2 }} />
                <Typography variant="body1">No Spanner instances found.</Typography>
              </Paper>
            ) : (
              <List>
                {instances.map((inst) => {
                  const instId = inst.name.split('/').pop();
                  const dbs = databasesByInstance[instId] || [];
                  return (
                    <Paper key={inst.name} sx={{ mb: 2, overflow: 'hidden', border: '1px solid #dadce0' }} elevation={0}>
                      <ListItem sx={{ bgcolor: '#fff' }}>
                        <ListItemIcon><StorageIcon color="primary" /></ListItemIcon>
                        <ListItemText 
                          primary={<Typography sx={{ fontWeight: 500 }}>{inst.displayName || instId}</Typography>}
                          secondary={inst.state} 
                        />
                        <Button size="small" onClick={() => { setTargetInstance(instId); setNewDbOpen(true); }}>Add DB</Button>
                      </ListItem>
                      <Divider />
                      <List disablePadding sx={{ bgcolor: '#fafafa' }}>
                        {dbs.length === 0 ? (
                          <ListItem><Typography variant="caption" sx={{ color: '#5f6368', pl: 7 }}>No databases</Typography></ListItem>
                        ) : dbs.map(db => {
                          const dbId = db.name.split('/').pop();
                          const dbPath = `${instId}/${dbId}`;
                          const isExpanded = expandedDb === dbPath;
                          const tables = tablesByDb[dbPath];
                          
                          return (
                            <Box key={db.name}>
                              <ListItemButton 
                                sx={{ pl: 5 }} 
                                onClick={() => {
                                  if (isExpanded) setExpandedDb(null);
                                  else {
                                    setExpandedDb(dbPath);
                                    if (tables === undefined) loadTables(instId, dbId);
                                  }
                                }}
                              >
                                <ListItemIcon sx={{ minWidth: 32 }}>
                                  {isExpanded ? <ExpandMoreIcon /> : <ChevronRightIcon />}
                                </ListItemIcon>
                                <ListItemText primary={<Typography variant="body2">{dbId}</Typography>} />
                                <Box>
                                  <IconButton size="small" onClick={(e) => {
                                    e.stopPropagation();
                                    loadTables(instId, dbId);
                                  }} title="Refresh Schema">
                                    <RefreshIcon sx={{ fontSize: 14 }} />
                                  </IconButton>
                                  <IconButton size="small" onClick={(e) => {
                                    e.stopPropagation();
                                    setSelectedDb(dbPath);
                                    setTabValue(1);
                                  }} title="Open SQL Workspace">
                                    <TerminalIcon sx={{ fontSize: 16 }} />
                                  </IconButton>
                                </Box>
                              </ListItemButton>
                              <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                                <List disablePadding sx={{ pl: 9, bgcolor: '#f1f3f4' }}>
                                  {tables === undefined ? (
                                    <ListItem><Typography variant="caption">Loading tables...</Typography></ListItem>
                                  ) : tables.length === 0 ? (
                                    <ListItem><Typography variant="caption" sx={{ color: '#5f6368' }}>No tables found</Typography></ListItem>
                                  ) : tables.map(t => (
                                    <ListItem key={t} sx={{ py: 0.5 }}>
                                      <ListItemIcon sx={{ minWidth: 28 }}><TableRowsIcon sx={{ fontSize: 14 }} /></ListItemIcon>
                                      <ListItemText primary={<Typography variant="caption" sx={{ fontWeight: 500 }}>{t}</Typography>} />
                                    </ListItem>
                                  ))}
                                </List>
                              </Collapse>
                            </Box>
                          );
                        })}
                      </List>
                    </Paper>
                  );
                })}
              </List>
            )}

            <Box sx={{ mt: 4 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                <SecurityIcon sx={{ fontSize: 18, color: '#5f6368' }} />
                <Typography variant="caption" sx={{ color: '#5f6368', fontWeight: 600 }}>Access Control (IAM)</Typography>
              </Box>
              <Typography variant="caption" sx={{ display: 'block', mb: 1, color: '#5f6368' }}>
                MiniSky validates all requests against the local IAM policy engine. 
                Use <code>setIamPolicy</code> to restrict access to specific service accounts.
              </Typography>
              <Paper sx={{ p: 2, bgcolor: '#202124', color: '#e8eaed', fontFamily: 'monospace', fontSize: '0.75rem' }}>
                gcloud config set auth/disable_credentials true<br/>
                gcloud config set api_endpoint_overrides/spanner http://localhost:8080/
              </Paper>
            </Box>
          </Box>
        )}

        {tabValue === 1 && (
          <Box sx={{ p: 3, flexGrow: 1, display: 'flex', flexDirection: 'column' }}>
            <Box sx={{ mb: 2, display: 'flex', gap: 2, alignItems: 'center' }}>
              <TextField
                select
                label="Target Database"
                value={selectedDb}
                onChange={(e) => setSelectedDb(e.target.value)}
                slotProps={{ select: { native: true } }}
                size="small"
                sx={{ flexGrow: 1 }}
              >
                <option value="">Select a database...</option>
                {Object.entries(databasesByInstance).flatMap(([instId, dbs]) => 
                  dbs.map(db => {
                    const dbId = db.name.split('/').pop();
                    return <option key={db.name} value={`${instId}/${dbId}`}>{instId} / {dbId}</option>
                  })
                )}
              </TextField>
              <Button 
                variant="contained" 
                startIcon={queryLoading ? <CircularProgress size={16} color="inherit" /> : <PlayArrowIcon />}
                onClick={executeSql}
                disabled={!selectedDb || queryLoading}
              >
                Run
              </Button>
            </Box>

            <TextField
              multiline
              rows={8}
              fullWidth
              variant="outlined"
              placeholder="Enter SQL (e.g. SELECT * FROM User;)"
              value={sqlQuery}
              onChange={(e) => setSqlQuery(e.target.value)}
              sx={{ bgcolor: 'white', mb: 2 }}
              slotProps={{ input: { sx: { fontFamily: 'monospace', fontSize: '0.9rem' } } }}
            />

            <Box sx={{ flexGrow: 1, overflow: 'auto' }}>
              {queryResults ? (
                <TableContainer component={Paper} elevation={0} sx={{ border: '1px solid #dadce0' }}>
                  <Table size="small" stickyHeader>
                    <TableHead>
                      <TableRow>
                        {queryResults.metadata?.rowType?.fields?.map((f: any) => (
                          <TableCell key={f.name} sx={{ bgcolor: '#f8f9fa', fontWeight: 600 }}>{f.name}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {queryResults.rows?.map((row: any, i: number) => (
                        <TableRow key={i}>
                          {row.map((cell: any, j: number) => (
                            <TableCell key={j}>{JSON.stringify(cell)}</TableCell>
                          ))}
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  {(!queryResults.rows || queryResults.rows.length === 0) && (
                    <Box sx={{ p: 4, textAlign: 'center', color: '#5f6368' }}>{queryResults.message || 'No results or empty set.'}</Box>
                  )}
                </TableContainer>
              ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', color: '#dadce0' }}>
                  <TerminalIcon sx={{ fontSize: 64, mb: 1 }} />
                  <Typography>Run a query to see results</Typography>
                </Box>
              )}
            </Box>
          </Box>
        )}
      </Box>

      {/* Dialogs */}
      <Dialog open={newInstanceOpen} onClose={() => setNewInstanceOpen(false)}>
        <DialogTitle>Create Spanner Instance</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Instance ID" fullWidth variant="outlined" value={newInstanceId} onChange={(e) => setNewInstanceId(e.target.value)} placeholder="e.g. my-instance" sx={{ mt: 1 }} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewInstanceOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateInstance} variant="contained" disabled={!newInstanceId}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={newDbOpen} onClose={() => setNewDbOpen(false)}>
        <DialogTitle>Create Database in {targetInstance}</DialogTitle>
        <DialogContent>
          <TextField autoFocus margin="dense" label="Database ID" fullWidth variant="outlined" value={newDbId} onChange={(e) => setNewDbId(e.target.value)} placeholder="e.g. my-database" sx={{ mt: 1 }} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setNewDbOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateDatabase} variant="contained" disabled={!newDbId}>Create</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={4000} onClose={() => setToast({ ...toast, open: false })} message={toast.msg} />
    </Drawer>
  );
}
