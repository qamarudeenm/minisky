import { Box, Typography, Card, CardContent, Chip, Button } from '@mui/material';
import CloudIcon from '@mui/icons-material/CloudOutlined';
import DnsIcon from '@mui/icons-material/Dns';
import MemoryIcon from '@mui/icons-material/Memory';
import SecurityIcon from '@mui/icons-material/Security';
import StorageIcon from '@mui/icons-material/Storage';
import LocalFireDepartmentIcon from '@mui/icons-material/LocalFireDepartment';
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch';
import LockIcon from '@mui/icons-material/LockOutlined';
import PsychologyIcon from '@mui/icons-material/Psychology';
import { Service } from '../hooks/useServices';

type ServiceCardProps = {
  service: Service;
  idx: number;
  settings: any;
  onStartContainer: (id: string) => void;
  onToggleSetting: (key: string, currentVal: boolean) => void;
  onManage?: (id: string) => void;
  onStopContainer?: (id: string) => void;
  onInstallDependency?: (id: string) => void;
};

export default function ServiceCard({ 
  service: s, idx, settings, onStartContainer, onStopContainer, onToggleSetting, onManage, onInstallDependency 
}: ServiceCardProps) {
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
      case 'bigtable': return <StorageIcon sx={{ color: '#e67c73', fontSize: '1.2rem' }}/>;
      case 'datastore': return <StorageIcon sx={{ color: '#f9ab00', fontSize: '1.2rem' }}/>;
      case 'spanner': return <StorageIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/>;
      case 'appengine': return <RocketLaunchIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/>;
      case 'memorystore': return <StorageIcon sx={{ color: '#c5221f', fontSize: '1.2rem' }}/>;
      case 'artifactregistry': return <StorageIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/>;
      case 'vertexai': return <PsychologyIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/>;
      case 'secretmanager': return <LockIcon sx={{ color: '#1e8e3e', fontSize: '1.2rem' }}/>;
      case 'cloudtasks': return <RocketLaunchIcon sx={{ color: '#1a73e8', fontSize: '1.2rem' }}/>;
      case 'firebase-auth':
      case 'firebase-rtdb':
      case 'firebase-hosting':
        return <LocalFireDepartmentIcon sx={{ color: '#ffca28', fontSize: '1.2rem' }}/>;
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
          {['storage', 'pubsub', 'firestore', 'bigtable', 'datastore', 'spanner', 'firebase-auth', 'firebase-rtdb', 'firebase-hosting'].includes(s.id) && s.status === 'SLEEPING' && (
            <Button size="small" variant="outlined" onClick={() => onStartContainer(s.id)}>Spin Up Container</Button>
          )}

          {s.id === 'gke' && s.status === 'SLEEPING' && s.missingDeps?.includes('kind') && onInstallDependency && (
            <Button size="small" variant="contained" color="warning" onClick={() => onInstallDependency('kind')} sx={{ fontWeight: 600 }}>Fix Missing Tool (kind)</Button>
          )}

          {s.status === 'SLEEPING' && s.missingDeps?.some(d => d.startsWith('docker-image:')) && onInstallDependency && (
            <Button 
              size="small" 
              variant="contained" 
              color="info" 
              onClick={() => onInstallDependency(s.missingDeps!.find(d => d.startsWith('docker-image:'))!)} 
              sx={{ fontWeight: 600 }}
            >
              Pull Required Image
            </Button>
          )}

          {['storage', 'pubsub', 'firestore', 'bigtable', 'datastore', 'spanner', 'firebase-auth', 'firebase-rtdb', 'firebase-hosting'].includes(s.id) && s.status === 'RUNNING' && onStopContainer && (
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

          {s.id === 'cloudfunctions' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Functions</Button>
          )}

          {s.id === 'serverless' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Services</Button>
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

          {s.id === 'bigtable' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Bigtable</Button>
          )}

          {s.id === 'datastore' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Datastore</Button>
          )}

          {s.id === 'memorystore' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Memorystore</Button>
          )}

          {s.id === 'scheduler' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Jobs</Button>
          )}

          {s.id === 'secretmanager' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Secrets</Button>
          )}

          {s.id === 'cloudkms' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage KMS</Button>
          )}

          {s.id === 'cloudbuild' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Builds</Button>
          )}

          {s.id === 'artifactregistry' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Registry</Button>
          )}

          {s.id === 'vertexai' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage AI Models</Button>
          )}
          
          {s.id === 'cloudtasks' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Tasks</Button>
          )}

          {s.id === 'spanner' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Spanner</Button>
          )}

          {s.id === 'gke' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage Clusters</Button>
          )}

          {s.id === 'appengine' && s.status === 'RUNNING' && onManage && (
            <Button size="small" variant="contained" color="secondary" onClick={() => onManage(s.id)}>Manage App Engine</Button>
          )}

          {s.id === 'serverless' && s.missingDeps?.includes('pack') && onInstallDependency && (
            <Button size="small" variant="contained" color="warning" onClick={() => onInstallDependency('pack')} sx={{ fontWeight: 600 }}>Fix Missing Tool (pack)</Button>
          )}
          
          {s.id === 'serverless' && (!s.missingDeps?.includes('pack')) && (
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
              {settings?.gke_kind ? 'Disable GKE Cluster' : 'Enable GKE Cluster'}
            </Button>
          )}
        </Box>
      </CardContent>
    </Card>
  );
}
