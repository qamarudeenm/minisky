import { useState, useEffect } from 'react';
import { 
  Drawer, Box, Typography, TextField, Button, 
  Stack, Divider, Alert, List, ListItem, ListItemText, ListItemSecondaryAction, IconButton,
  CircularProgress, Collapse, Paper, Chip
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ExpandLessIcon from '@mui/icons-material/ExpandLess';
import { useProjectContext } from '../contexts/ProjectContext';

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
}

interface Props {
  open: boolean;
  onClose: () => void;
}

export default function CloudTasksDrawer({ open, onClose }: Props) {
  const { activeProject } = useProjectContext();
  const [queues, setQueues] = useState<Queue[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const [newQueueId, setNewQueueId] = useState('');
  const [creating, setCreating] = useState(false);

  const [expandedQueue, setExpandedQueue] = useState<string | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loadingTasks, setLoadingTasks] = useState(false);

  // Locations are usually just 'us-central1' in emulators
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
    if (open) {
      fetchQueues();
    } else {
      setExpandedQueue(null);
      setTasks([]);
    }
  }, [open, activeProject]);

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

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ width: 550, p: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h5" sx={{ fontWeight: 500 }}>Cloud Tasks</Typography>
          <IconButton onClick={onClose}><CloseIcon /></IconButton>
        </Box>

        <Divider sx={{ mb: 4 }} />

        {error && <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>{error}</Alert>}

        <Paper variant="outlined" sx={{ mb: 6, p: 3, background: '#f8f9fa' }}>
          <Typography variant="subtitle2" sx={{ mb: 2, fontWeight: 600 }}>Create New Queue</Typography>
          <Stack direction="row" spacing={2}>
            <TextField 
              label="Queue ID" 
              size="small" 
              fullWidth 
              value={newQueueId}
              onChange={(e) => setNewQueueId(e.target.value)}
              placeholder="e.g. email-tasks"
            />
            <Button 
              variant="contained" 
              startIcon={<AddIcon />}
              onClick={handleCreateQueue}
              disabled={creating || !newQueueId}
              sx={{ minWidth: '140px' }}
            >
              {creating ? 'Creating...' : 'Create Queue'}
            </Button>
          </Stack>
        </Paper>

        <Typography variant="h6" sx={{ mb: 2, fontSize: '1.1rem' }}>Queues in {activeProject}</Typography>
        
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress size={24} />
          </Box>
        ) : (
          <List disablePadding>
            {queues.length === 0 ? (
              <Typography variant="body2" sx={{ color: '#70757a', textAlign: 'center', py: 4 }}>
                No queues found in this project.
              </Typography>
            ) : (
              queues.map((q) => {
                const id = q.name.split('/').pop();
                const isExpanded = expandedQueue === q.name;
                return (
                  <Box key={q.name} sx={{ mb: 1, borderBottom: '1px solid #eee' }}>
                    <ListItem sx={{ px: 1 }}>
                      <IconButton size="small" onClick={() => toggleExpand(q.name)} sx={{ mr: 1 }}>
                        {isExpanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
                      </IconButton>
                      <ListItemText 
                        primary={<Typography sx={{ fontWeight: 500 }}>{id}</Typography>}
                        secondary={`State: ${q.state}`}
                      />
                      <ListItemSecondaryAction>
                        <IconButton size="small" color="error" onClick={() => handleDeleteQueue(q.name)}>
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </ListItemSecondaryAction>
                    </ListItem>
                    
                    <Collapse in={isExpanded} timeout="auto" unmountOnExit>
                      <Box sx={{ pl: 6, pr: 2, pb: 2 }}>
                        <Typography variant="subtitle2" sx={{ mb: 1, fontSize: '0.8rem', color: '#5f6368' }}>Pending Tasks</Typography>
                        {loadingTasks ? (
                          <CircularProgress size={16} sx={{ my: 1 }} />
                        ) : (
                          <Stack spacing={1}>
                            {tasks.length === 0 ? (
                              <Typography variant="caption" sx={{ color: '#999', py: 1 }}>No pending tasks in this queue.</Typography>
                            ) : (
                              tasks.map((t) => {
                                const tId = t.name.split('/').pop();
                                return (
                                  <Box key={t.name} sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', bgcolor: '#f1f3f4', p: 1, borderRadius: '4px' }}>
                                    <Box>
                                      <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.8rem' }}>{tId}</Typography>
                                      {t.httpRequest && (
                                        <Typography variant="caption" color="textSecondary">
                                          {t.httpRequest.httpMethod} {t.httpRequest.url}
                                        </Typography>
                                      )}
                                    </Box>
                                    <Chip size="small" label="Pending" color="primary" variant="outlined" />
                                  </Box>
                                );
                              })
                            )}
                          </Stack>
                        )}
                      </Box>
                    </Collapse>
                  </Box>
                );
              })
            )}
          </List>
        )}
      </Box>
    </Drawer>
  );
}
