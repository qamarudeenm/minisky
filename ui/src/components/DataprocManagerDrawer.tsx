import { useState, useEffect, useCallback } from 'react';
import {
  Drawer, Box, Typography, IconButton, Button,
  Snackbar, Alert, Dialog,
  DialogTitle, DialogContent, DialogActions, Paper,
  TextField, Chip, Tabs, Tab
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import { useProjectContext } from '../contexts/ProjectContext';

type DataprocManagerDrawerProps = { open: boolean; onClose: () => void };

export default function DataprocManagerDrawer({ open, onClose }: DataprocManagerDrawerProps) {
  const { activeProject } = useProjectContext();
  const [clusters, setClusters] = useState<any[]>([]);
  const [jobs, setJobs] = useState<any[]>([]);
  const [activeTab, setActiveTab] = useState(0);

  const [toast, setToast] = useState({ msg: '', open: false, severity: 'success' as 'success' | 'error' });

  // Cluster Dialog
  const [newClusterOpen, setNewClusterOpen] = useState(false);
  const [newClusterName, setNewClusterName] = useState('');
  const [numWorkers, setNumWorkers] = useState(2);

  // Job Dialog
  const [newJobOpen, setNewJobOpen] = useState(false);
  const [jobScriptUri, setJobScriptUri] = useState('gs://my-bucket/script.py');
  const [targetCluster, setTargetCluster] = useState('');

  const region = 'us-central1';
  const apiRoot = `/api/manage/dataproc/projects/${activeProject}/regions/${region}`;

  const showToast = (msg: string, severity: 'success' | 'error' = 'success') =>
    setToast({ msg, open: true, severity });

  const loadData = useCallback(async () => {
    try {
      const [resC, resJ] = await Promise.all([
        fetch(`${apiRoot}/clusters`),
        fetch(`${apiRoot}/jobs`)
      ]);
      if (resC.ok) {
        const data = await resC.json();
        setClusters(data.clusters || []);
      }
      if (resJ.ok) {
        const data = await resJ.json();
        setJobs(data.jobs || []);
      }
    } catch (e) { console.error(e); }
  }, [apiRoot]);

  useEffect(() => {
    if (open) loadData();
  }, [open, loadData]);

  const handleCreateCluster = async () => {
    if (!newClusterName) return;
    try {
      const payload = {
        clusterName: newClusterName,
        config: {
          masterConfig: { numInstances: 1, machineTypeUri: 'n1-standard-4' },
          workerConfig: { numInstances: numWorkers, machineTypeUri: 'n1-standard-4' }
        }
      };
      const res = await fetch(`${apiRoot}/clusters`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        showToast('Provisioning Spark/Hadoop cluster nodes...');
        setTimeout(loadData, 1000);
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewClusterOpen(false);
    setNewClusterName('');
  };

  const handleDeleteCluster = async (name: string) => {
    await fetch(`${apiRoot}/clusters/${name}`, { method: 'DELETE' });
    showToast('Tearing down cluster nodes...');
    setTimeout(loadData, 1000);
  };

  const handleSubmitJob = async () => {
    if (!targetCluster || !jobScriptUri) return;
    try {
      const payload = {
        job: {
          placement: { clusterName: targetCluster },
          pysparkJob: { mainPythonFileUri: jobScriptUri, args: [] }
        }
      };
      const res = await fetch(`${apiRoot}/jobs:submit`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      if (res.ok) {
        showToast('PySpark Job submitted successfully.');
        setTimeout(loadData, 500);
      } else {
        const e = await res.json();
        showToast(e.error?.message || 'Failed', 'error');
      }
    } catch (e: any) { showToast(e.message, 'error'); }
    setNewJobOpen(false);
  };

  return (
    <>
      <Drawer anchor="right" open={open} onClose={onClose}>
        <Box sx={{ display: 'flex', flexDirection: 'column', height: '100vh', width: '75vw', maxWidth: 1000, bgcolor: '#f8f9fa' }}>
          
          <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'white', borderBottom: '1px solid #dadce0' }}>
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 500, color: '#1a73e8' }}>Cloud Dataproc</Typography>
              <Typography variant="caption" sx={{ color: '#5f6368' }}>Native Apache Spark & Hadoop Orchestration • {activeProject}</Typography>
            </Box>
            <Box>
              <IconButton onClick={() => loadData()} size="small" sx={{ mr: 1 }}><RefreshIcon /></IconButton>
              <IconButton onClick={onClose} size="small"><CloseIcon /></IconButton>
            </Box>
          </Box>

          <Box sx={{ borderBottom: 1, borderColor: 'divider', bgcolor: 'white', px: 3 }}>
            <Tabs value={activeTab} onChange={(_e, v) => setActiveTab(v)}>
              <Tab label="Clusters" />
              <Tab label="Jobs" />
            </Tabs>
          </Box>

          <Box sx={{ flex: 1, p: 4, overflow: 'auto' }}>
            {activeTab === 0 && (
              <>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 3 }}>
                  <Typography variant="h6">Clusters</Typography>
                  <Button variant="contained" startIcon={<AddIcon />} onClick={() => setNewClusterOpen(true)}>Create Cluster</Button>
                </Box>
                {clusters.length === 0 ? (
                  <Typography variant="body2" sx={{ color: '#80868b', fontStyle: 'italic', textAlign: 'center', mt: 4 }}>No active Dataproc clusters.</Typography>
                ) : (
                  <Box sx={{ display: 'grid', gap: 2 }}>
                    {clusters.map(cl => (
                      <Paper key={cl.clusterName} elevation={0} sx={{ p: 3, border: '1px solid #dadce0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <Box>
                          <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>{cl.clusterName}</Typography>
                          <Typography variant="body2" sx={{ color: '#5f6368', mb: 1 }}>
                            Master: 1 node • Workers: {cl.config?.workerConfig?.numInstances || 0} nodes
                          </Typography>
                          <Chip size="small" label={cl.status?.state} color={cl.status?.state === 'RUNNING' ? 'success' : 'warning'} />
                        </Box>
                        <IconButton color="error" onClick={() => handleDeleteCluster(cl.clusterName)}><DeleteIcon /></IconButton>
                      </Paper>
                    ))}
                  </Box>
                )}
              </>
            )}

            {activeTab === 1 && (
              <>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 3 }}>
                  <Typography variant="h6">Submitted Jobs</Typography>
                  <Button variant="contained" startIcon={<PlayArrowIcon />} onClick={() => setNewJobOpen(true)}>Submit Job</Button>
                </Box>
                {jobs.length === 0 ? (
                  <Typography variant="body2" sx={{ color: '#80868b', fontStyle: 'italic', textAlign: 'center', mt: 4 }}>No job history.</Typography>
                ) : (
                  <Box sx={{ display: 'grid', gap: 2 }}>
                    {jobs.map(j => (
                      <Paper key={j.reference?.jobId} elevation={0} sx={{ p: 2, border: '1px solid #dadce0' }}>
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                          <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>{j.reference?.jobId}</Typography>
                          <Chip size="small" label={j.status?.state} color={j.status?.state === 'DONE' ? 'success' : j.status?.state === 'ERROR' ? 'error' : 'primary'} />
                        </Box>
                        <Typography variant="body2" sx={{ color: '#5f6368' }}>Cluster: <strong>{j.placement?.clusterName}</strong></Typography>
                        <Typography variant="body2" sx={{ color: '#5f6368' }}>Entrypoint: <strong>{j.pysparkJob?.mainPythonFileUri || j.sparkJob?.mainJarFileUri}</strong></Typography>
                      </Paper>
                    ))}
                  </Box>
                )}
              </>
            )}
          </Box>
        </Box>
      </Drawer>

      <Dialog open={newClusterOpen} onClose={() => setNewClusterOpen(false)}>
        <DialogTitle>Provision Dataproc Spark Cluster</DialogTitle>
        <DialogContent sx={{ width: 400 }}>
          <TextField autoFocus margin="dense" label="Cluster Name" fullWidth variant="outlined"
                     value={newClusterName} onChange={e => setNewClusterName(e.target.value.replace(/[^a-zA-Z0-9-]/g, ''))} sx={{ mb: 3, mt: 1 }} />
          <TextField margin="dense" label="Number of Worker Nodes" fullWidth variant="outlined" type="number"
                     value={numWorkers} onChange={e => setNumWorkers(parseInt(e.target.value) || 0)} sx={{ mb: 1 }} />
          <Typography variant="caption" sx={{ color: '#5f6368' }}>Note: Each node provisions a physical bitnami/spark:3.5 container.</Typography>
        </DialogContent>
        <DialogActions sx={{ p: 2, pt: 0 }}>
          <Button onClick={() => setNewClusterOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={handleCreateCluster} disabled={!newClusterName}>Create</Button>
        </DialogActions>
      </Dialog>

      <Dialog open={newJobOpen} onClose={() => setNewJobOpen(false)}>
        <DialogTitle>Submit Dataproc PySpark Job</DialogTitle>
        <DialogContent sx={{ width: 450 }}>
          <TextField autoFocus margin="dense" label="Target Cluster Name" fullWidth variant="outlined"
                     value={targetCluster} onChange={e => setTargetCluster(e.target.value)} sx={{ mb: 3, mt: 1 }} />
          <TextField margin="dense" label="Main Python File URI (gs://...)" fullWidth variant="outlined"
                     value={jobScriptUri} onChange={e => setJobScriptUri(e.target.value)} />
        </DialogContent>
        <DialogActions sx={{ p: 2, pt: 0 }}>
          <Button onClick={() => setNewJobOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={handleSubmitJob} disabled={!targetCluster || !jobScriptUri}>Submit</Button>
        </DialogActions>
      </Dialog>

      <Snackbar open={toast.open} autoHideDuration={3000} onClose={() => setToast(t => ({ ...t, open: false }))}>
        <Alert severity={toast.severity}>{toast.msg}</Alert>
      </Snackbar>
    </>
  );
}
