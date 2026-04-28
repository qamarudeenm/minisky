import { useState, useEffect } from 'react';
import { 
  Box, Typography, TextField, Button, 
  Stack, Alert, IconButton,
  CircularProgress, Collapse, Paper, Chip, Table, TableBody, TableCell, TableContainer, TableHead, TableRow
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import RefreshIcon from '@mui/icons-material/Refresh';
import SendIcon from '@mui/icons-material/Send';
import { useProjectContext } from '../contexts/ProjectContext';
import { Dialog, DialogTitle, DialogContent, DialogActions } from '@mui/material';

interface Queue {
  name: string;
  state: string;
}

interface Task {
  name: string;
  httpRequest?: {
    url: string;
    httpMethod: string;
  };
  createTime: string;
  status?: string;
}

export default function CloudTasksPageContent() {
  const { activeProject } = useProjectContext();
  const [queues, setQueues] = useState<Queue[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const [newQueueId, setNewQueueId] = useState('');
  const [creating, setCreating] = useState(false);

  const [expandedQueue, setExpandedQueue] = useState<string | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loadingTasks, setLoadingTasks] = useState(false);

  // Create Task State
  const [taskDialogOpen, setTaskDialogOpen] = useState(false);
  const [targetQueue, setTargetQueue] = useState<string | null>(null);
  const [taskTargetUrl, setTaskTargetUrl] = useState('http://localhost:8080/my-endpoint');
  const [taskPayload, setTaskPayload] = useState('{"message": "hello"}');
  const [submittingTask, setSubmittingTask] = useState(false);

  const location = 'us-central1';

  const fetchQueues = async () => {
    setLoading(true);
    try {
      const res = await fetch(`/api/manage/cloudtasks/projects/${activeProject}/locations/${location}/queues`);
      if (!res.ok) throw new Error('Failed to fetch queues');
      const data = await res.json();
      setQueues(data.queues || []);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchTasks = async (queueName: string) => {
    setLoadingTasks(true);
    try {
      const res = await fetch(`/api/manage/cloudtasks/${queueName}/tasks`);
      if (!res.ok) throw new Error('Failed to fetch tasks');
      const data = await res.json();
      setTasks(data.tasks || []);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoadingTasks(false);
    }
  };

  useEffect(() => {
    fetchQueues();
  }, [activeProject]);

  const handleCreateQueue = async () => {
    setCreating(true);
    setError(null);
    try {
      const res = await fetch(`/api/manage/cloudtasks/projects/${activeProject}/locations/${location}/queues`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ 
          name: `projects/${activeProject}/locations/${location}/queues/${newQueueId}`
        })
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error?.message || 'Failed to create queue');
      }

      setNewQueueId('');
      fetchQueues();
    } catch (err: any) {
      setError(err.message);
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteQueue = async (name: string) => {
    const queueId = name.split('/').pop();
    if (!confirm(`Are you sure you want to delete queue ${queueId}?`)) return;
    
    try {
      const res = await fetch(`/api/manage/cloudtasks/${name}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('Failed to delete queue');
      if (expandedQueue === name) setExpandedQueue(null);
      fetchQueues();
    } catch (err: any) {
      setError(err.message);
    }
  };

  const toggleExpand = (name: string) => {
    if (expandedQueue === name) {
      setExpandedQueue(null);
      setTasks([]);
    } else {
      setExpandedQueue(name);
      fetchTasks(name);
    }
  };

  const handleCreateTask = async () => {
    if (!targetQueue) return;
    setSubmittingTask(true);
    try {
      const res = await fetch(`/api/manage/cloudtasks/${targetQueue}/tasks`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          task: {
            httpRequest: {
              url: taskTargetUrl,
              httpMethod: 'POST',
              body: btoa(taskPayload) // Cloud Tasks expects base64 body
            }
          }
        })
      });
      if (!res.ok) throw new Error('Failed to create task');
      setTaskDialogOpen(false);
      if (expandedQueue === targetQueue) fetchTasks(targetQueue);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setSubmittingTask(false);
    }
  };

  const handleDeleteTask = async (queueName: string, taskName: string) => {
    const taskId = taskName.split('/').pop();
    if (!confirm(`Delete task ${taskId}?`)) return;

    try {
      const res = await fetch(`/api/manage/cloudtasks/${taskName}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('Failed to delete task');
      fetchTasks(queueName);
    } catch (err: any) {
      setError(err.message);
    }
  };

  return (
    <Box>
      <Box sx={{ mb: 4, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h6" sx={{ fontSize: '1.2rem' }}>Queues in {activeProject}</Typography>
        <Box sx={{ display: 'flex', gap: 2 }}>
          <Button startIcon={<RefreshIcon />} onClick={fetchQueues}>Refresh</Button>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <TextField 
              label="New Queue ID" 
              size="small" 
              value={newQueueId}
              onChange={(e) => setNewQueueId(e.target.value)}
              sx={{ width: 200 }}
            />
            <Button 
              variant="contained" 
              startIcon={<AddIcon />}
              onClick={handleCreateQueue}
              disabled={creating || !newQueueId}
            >
              Create
            </Button>
          </Box>
        </Box>
      </Box>

      {error && <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>{error}</Alert>}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <CircularProgress />
        </Box>
      ) : (
        <TableContainer component={Paper} sx={{ borderRadius: '8px', border: '1px solid #dadce0', boxShadow: 'none' }}>
          <Table>
            <TableHead sx={{ backgroundColor: '#f8f9fa' }}>
              <TableRow>
                <TableCell sx={{ width: '40px' }} />
                <TableCell sx={{ fontWeight: 600 }}>Queue ID</TableCell>
                <TableCell sx={{ fontWeight: 600 }}>State</TableCell>
                <TableCell sx={{ fontWeight: 600 }} align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {queues.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={4} align="center" sx={{ py: 4, color: '#80868b' }}>
                    No queues found. Create one to get started.
                  </TableCell>
                </TableRow>
              ) : (
                queues.map((q) => {
                  const id = q.name.split('/').pop();
                  const isExpanded = expandedQueue === q.name;
                  return (
                    <>
                      <TableRow key={q.name} sx={{ '&:hover': { backgroundColor: '#fcfcfc' } }}>
                        <TableCell>
                          <IconButton size="small" onClick={() => toggleExpand(q.name)}>
                            {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                          </IconButton>
                        </TableCell>
                        <TableCell sx={{ fontWeight: 500 }}>{id}</TableCell>
                        <TableCell>
                          <Chip 
                            label={q.state} 
                            size="small" 
                            color={q.state === 'RUNNING' ? 'success' : 'default'}
                            sx={{ fontWeight: 500, fontSize: '0.75rem' }}
                          />
                        </TableCell>
                        <TableCell align="right">
                          <Button 
                            size="small" 
                            variant="outlined" 
                            startIcon={<SendIcon />} 
                            sx={{ mr: 1 }}
                            onClick={() => { setTargetQueue(q.name); setTaskDialogOpen(true); }}
                          >
                            Create Task
                          </Button>
                          <IconButton onClick={() => handleDeleteQueue(q.name)} color="error"><DeleteIcon /></IconButton>
                        </TableCell>
                      </TableRow>
                      <TableRow>
                        <TableCell colSpan={4} sx={{ py: 0, borderBottom: isExpanded ? '1px solid #eee' : 'none' }}>
                          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                            <Box sx={{ m: 2, ml: 8 }}>
                              <Typography variant="subtitle2" sx={{ mb: 2, fontSize: '0.85rem', color: '#5f6368' }}>Pending Tasks in {id}</Typography>
                              {loadingTasks ? (
                                <CircularProgress size={20} sx={{ my: 2 }} />
                              ) : (
                                <Stack spacing={1}>
                                  {tasks.length === 0 ? (
                                    <Typography variant="body2" sx={{ color: '#999', py: 2 }}>No pending tasks in this queue.</Typography>
                                  ) : (
                                    tasks.map((t) => {
                                      const tId = t.name.split('/').pop();
                                      return (
                                        <Paper variant="outlined" key={t.name} sx={{ p: 2, bgcolor: '#f8f9fa' }}>
                                          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                            <Box>
                                              <Typography variant="body2" sx={{ fontWeight: 600 }}>{tId}</Typography>
                                              {t.httpRequest && (
                                                <Typography variant="caption" color="textSecondary" sx={{ fontFamily: 'monospace' }}>
                                                  {t.httpRequest.httpMethod} {t.httpRequest.url}
                                                </Typography>
                                              )}
                                            </Box>
                                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                              <Chip size="small" label={t.status || 'PENDING'} color={t.status === 'COMPLETED' ? 'success' : 'primary'} />
                                              <IconButton size="small" onClick={() => handleDeleteTask(q.name, t.name)} color="error">
                                                <DeleteIcon fontSize="small" />
                                              </IconButton>
                                            </Box>
                                          </Box>
                                        </Paper>
                                      );
                                    })
                                  )}
                                </Stack>
                              )}
                            </Box>
                          </Collapse>
                        </TableCell>
                      </TableRow>
                    </>
                  );
                })
              )}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {/* Create Task Dialog */}
      <Dialog open={taskDialogOpen} onClose={() => setTaskDialogOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>Create New Task in Queue</DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ mt: 1 }}>
            <TextField
              label="Target URL"
              fullWidth
              value={taskTargetUrl}
              onChange={(e) => setTaskTargetUrl(e.target.value)}
              helperText="The local endpoint this task will POST to"
            />
            <TextField
              label="JSON Payload"
              fullWidth
              multiline
              rows={4}
              value={taskPayload}
              onChange={(e) => setTaskPayload(e.target.value)}
              sx={{ fontFamily: 'monospace' }}
            />
          </Stack>
        </DialogContent>
        <DialogActions sx={{ p: 3 }}>
          <Button onClick={() => setTaskDialogOpen(false)}>Cancel</Button>
          <Button 
            variant="contained" 
            onClick={handleCreateTask} 
            disabled={submittingTask}
            startIcon={<SendIcon />}
          >
            {submittingTask ? 'Dispatching...' : 'Dispatch Task'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
