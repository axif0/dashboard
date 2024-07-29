import Panel from '@/components/panel';
import Scheduleattemptstotal from './schedule_attempts_total';
import ClusterInfo from './cluster-info';
 
const KarmadaMetricsReference = () => {
  return (
    <Panel>
      <Scheduleattemptstotal/>
      <ClusterInfo/>
    </Panel>
  );
};

export default KarmadaMetricsReference;
