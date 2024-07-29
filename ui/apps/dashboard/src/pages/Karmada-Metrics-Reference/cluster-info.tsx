import i18nInstance from '@/utils/i18n';
import Panel from '@/components/panel';
import { useQuery } from '@tanstack/react-query';
import { GetClusters } from '@/services';
import {
  Cluster,
  ClusterDetail,
  DeleteCluster,
  GetClusterDetail,
} from '@/services/cluster';
import {
  Badge,
  Tag,
  Table,
  TableColumnProps,
  Progress,
  message
} from 'antd';
 
const ClusterInfo = () => {

function getPercentColor(v: number): string {
    // 0~60 #52C41A
    // 60~80 #FAAD14
    // > 80 #F5222D
    if (v <= 60) {
        return '#52C41A';
    } else if (v <= 80) {
        return '#FAAD14';
    } else {
        return '#F5222D';
    }
    }
    
const [messageApi, messageContextHolder] = message.useMessage();
const { data, isLoading, refetch } = useQuery({
    queryKey: ['GetClusters'],
    queryFn: async () => {
        const ret = await GetClusters();
        return ret.data;
    },
    });



    const columns: TableColumnProps<Cluster>[] = [
        {
          title: "Cluster Name",
          key: 'clusterName',
          width: 150,
          render: (_, r) => {
            r.ready;
            return r.objectMeta.name;
          },
        },
        {
          title: i18nInstance.t('bd17297989ec345cbc03ae0b8a13dc0a'),
          dataIndex: 'kubernetesVersion',
          key: 'kubernetesVersion',
          width: 150,
          align: 'center',
        },
        {
          title: i18nInstance.t('ee00813361387a116d274c608ba8bb13'),
          dataIndex: 'ready',
          key: 'ready',
          align: 'center',
          width: 150,
          render: (v) => {
            if (v) {
              return (
                <Badge
                  color={'green'}
                  text={<span style={{ color: '#52c41a' }}>ready</span>}
                />
              );
            } else {
              return (
                <Badge
                  color={'red'}
                  text={<span style={{ color: '#f5222d' }}>not ready</span>}
                />
              );
            }
          },
        },
        {
          title: i18nInstance.t('f0789e79d48f135e5d870753f7a85d05'),
          dataIndex: 'syncMode',
          width: 150,
          align: 'center',
          render: (v) => {
            if (v === 'Push') {
              return <Tag color={'gold'}>{v}</Tag>;
            } else {
              return <Tag color={'blue'}>{v}</Tag>;
            }
          },
        },
        {
          title: i18nInstance.t('b86224e030e5948f96b70a4c3600b33f'),
          dataIndex: 'nodeStatus',
          align: 'center',
          width: 150,
          render: (_, r) => {
            if (r.nodeSummary) {
              const { totalNum, readyNum } = r.nodeSummary;
              return (
                <>
                  {readyNum}/{totalNum}
                </>
              );
            }
            return '-';
          },
        },
        {
          title: i18nInstance.t('763a78a5fc84dbca6f0137a591587f5f'),
          dataIndex: 'cpuFraction',
          width: '15%',
          render: (_, r) => {
            const fraction = parseFloat(
              r.allocatedResources.cpuFraction.toFixed(2),
            );
            return (
              <Progress
                percent={fraction}
                strokeColor={getPercentColor(fraction)}
              />
            );
          },
        },
        {
          title: i18nInstance.t('8b2e672e8b847415a47cc2dd25a87a07'),
          dataIndex: 'memoryFraction',
          width: '15%',
          render: (_, r) => {
            const fraction = parseFloat(
              r.allocatedResources.memoryFraction.toFixed(2),
            );
            return (
              <Progress
                percent={fraction}
                strokeColor={getPercentColor(fraction)}
              />
            );
          },
        },
       
      ];
 


      return (
        <Panel>
        <h1 className="text-3xl font-bold mb-4">Cluster Information</h1>

          <Table
            rowKey={(r: Cluster) => r.objectMeta.name || ''}
            columns={columns}
            loading={isLoading}
            dataSource={data?.clusters || []}
          />

          {messageContextHolder}
        </Panel>
      );





















};

export default ClusterInfo;