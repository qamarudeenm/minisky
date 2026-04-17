import { Box, Typography, Card, CardContent, Chip, Button } from '@mui/material';
import CloudIcon from '@mui/icons-material/CloudOutlined';
import DnsIcon from '@mui/icons-material/Dns';
import MemoryIcon from '@mui/icons-material/Memory';
import SecurityIcon from '@mui/icons-material/Security';
import StorageIcon from '@mui/icons-material/Storage';
import { Service } from '../hooks/useServices';

type ServiceCardProps = {
  service: Service;
  idx: number;
  settings: any;
  onStartContainer: (id: string) => void;
  onToggleSetting: (key: string, currentVal: boolean) => void;
  onManage?: (id: string) => void;
  onStopContainer?: (id: string) => void;
};

export default function ServiceCard({ service: s, idx, settings, onStartContainer, onStopContainer, onToggleSetting, onManage }: ServiceCardProps) {
  const getIcon = () => {
    switch (s.id) {
      case 'storage': return <CloudIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/>;
      case 'pubsub': return <DnsIcon sx={{ color: '#f9ab00', fontSize: '1.2rem' }}/>;
      case 'firestore': return <StorageIcon sx={{ color: '#f9ab00', fontSize: '1.2rem' }}/>;
      case 'compute': return <MemoryIcon sx={{ color: '#d93025', fontSize: '1.2rem' }}/>;
      case 'gke': return <CloudIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/>;
      case 'bigquery': return <StorageIcon sx={{ color: '#f9ab00', fontSize: '1.2rem' }}/>;
      case 'sqladmin': return <StorageIcon sx={{ color: '#f9ab00', fontSize: '1.2rem' }}/>;
      case 'serverless': return <MemoryIcon sx={{ color: '#d93025', fontSize: '1.2rem' }}/>;
      case 'dns': return <DnsIcon sx={{ color: '#f9ab00', fontSize: '1.2rem' }}/>;
      case 'iam': return <SecurityIcon sx={{ color: '#1e8e3e', fontSize: '1.2rem' }}/>;
      case 'dataproc': return <MemoryIcon sx={{ color: '#d93025', fontSize: '1.2rem' }}/>;
      default:
        return idx % 4 === 0 ? <CloudIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/> :
               idx % 4 === 1 ? <MemoryIcon sx={{ color: '#d93025', fontSize: '1.2rem' }}/> :
               idx % 4 === 2 ? <DnsIcon sx={{ color: '#f9ab00', fontSize: '1.2rem' }}/> :
               <SecurityIcon sx={{ color: '#1e8e3e', fontSize: '1.2rem' }}/>;
    }
  };

  return (
    <Card className="material-panel" sx={{ height: '100%' }}>
      <CardContent sx={{ p: 3, display: 'flex', flexDirection: 'column', height: '100%' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <Box sx={{ 
              p: 1.5, 
              borderRadius: '8px', 
              background: '#f8f9fa',
              border: '1px solid #dadce0',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center'
            }}>
              {getIcon()}
            </Box>
            <Typography variant="h6" sx={{ fontWeight: 500, fontSize: '1.1rem' }}>{s.label}</Typography>
          </Box>
          <Chip 
            label={s.status} 
            className={s.status === 'RUNNING' ? 'status-indicator-running' : 'status-indicator-sleeping'}
            sx={{ 
              fontWeight: 600,
              fontSize: '0.65rem',
              height: '24px'
            }} 
          />
        </Box>
        <Typography variant="body2" sx={{ color: '#5f6368', lineHeight: 1.5, mb: 3, flexGrow: 1 }}>
          {s.description}
        </Typography>
        
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          {s.port ? (
            <Box sx={{ display: 'inline-flex', alignItems: 'center', background: '#f1f3f4', border: '1px solid #dadce0', px: 1.5, py: 0.5, borderRadius: '4px', alignSelf: 'flex-start' }}>
              <Typography variant="caption" sx={{ fontFamily: 'monospace', color: '#1a73e8', fontWeight: 600 }}>
                tcp://localhost:{s.port}
              </Typography>
            </Box>
          ) : (
            <Box sx={{ display: 'inline-flex', alignItems: 'center', px: 1, py: 0.5, alignSelf: 'flex-start', border: '1px dashed #dadce0', borderRadius: '4px' }}>
              <Typography variant="caption" sx={{ color: '#80868b' }}>
                Proxy routing active on API gateway
              </Typography>
            </Box>
          )}
          
          {/* Dynamic Controls based on service ID */}
          {['storage', 'pubsub', 'firestore'].includes(s.id) && s.status === 'SLEEPING' && (
            <Button size="small" variant="outlined" onClick={() => onStartContainer(s.id)}>Spin Up Container</Button>
          )}

          {['storage', 'pubsub', 'firestore'].includes(s.id) && s.status === 'RUNNING' && onStopContainer && (
            <Button size="small" variant="outlined" color="error" onClick={() => onStopContainer(s.id)}>Stop Container</Button>
          )}

          {s.id === 'firestore' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Database</Button>
          )}

          {s.id === 'pubsub' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Sandbox</Button>
          )}

          {s.id === 'bigquery' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Query Workspace</Button>
          )}

          {s.id === 'sqladmin' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Instance</Button>
          )}

          {s.id === 'storage' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Storage</Button>
          )}

          {s.id === 'dataproc' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Clusters</Button>
          )}

          {s.id === 'iam' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage IAM</Button>
          )}

          {s.id === 'compute' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Compute</Button>
          )}

          {s.id === 'dns' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Network</Button>
          )}
          
          {s.id === 'serverless' && (
            <Button size="small" variant="outlined" color={settings?.serverless_pack ? 'error' : 'primary'} onClick={() => onToggleSetting('serverless_pack', settings?.serverless_pack)}>
              {settings?.serverless_pack ? 'Disable Buildpacks' : 'Enable Buildpacks'}
            </Button>
          )}
          
          {s.id === 'bigquery' && (
            <Button size="small" variant="outlined" color={settings?.bq_duckdb ? 'error' : 'primary'} onClick={() => onToggleSetting('bq_duckdb', settings?.bq_duckdb)}>
              {settings?.bq_duckdb ? 'Disable DuckDB' : 'Enable DuckDB'}
            </Button>
          )}
          
          {s.id === 'gke' && (
            <Button size="small" variant="outlined" color={settings?.gke_kind ? 'error' : 'primary'} onClick={() => onToggleSetting('gke_kind', settings?.gke_kind)}>
              {settings?.gke_kind ? 'Disable Kind Cluster' : 'Enable Kind Cluster'}
            </Button>
          )}
        </Box>
      </CardContent>
    </Card>
  );
}
