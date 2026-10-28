import test from 'node:test';
import assert from 'node:assert/strict';
import { graphSegments, historyView } from './analytics-model.mjs';
import { fuelView } from './fuel.mjs';

test('history uses saved source and separates gaps without inventing points', () => {
 const at = n => new Date(n*1000).toISOString();
 const lines=graphSegments([{at:at(0),value:0},{at:at(1),value:2},{at:at(2),value:null},{at:at(3),value:4},{at:at(4),value:5,break:true}]);
 assert.deepEqual(lines.map(line=>line.map(p=>p.value)),[[0,2],[4],[5]]);
 const view=historyView({source:'live',liters:.9369,distance_km:9.569,average_l100:9.79,incomplete:true,retention_truncated:true,downsampled:true,last:{sample:{fuel:{quality:'DEGRADED',method:'speed_density',factor:1}}}});
 assert.equal(view.source,'LIVE · ИСТОРИЯ');
 assert.match(view.note,/непол|пропуск/i);
 assert.match(view.note,/ротац/i);
 assert.match(view.note,/прореж/i);
 assert.match(view.model,/DEGRADED/);
 assert.match(view.liters,/0.937/);
 assert.equal(historyView(null).source,'UNKNOWN · ИСТОРИЯ');
});

test('bad values and reversed times cannot create graph bridges', () => {
 const points=[{at:'2026-10-28T10:00:02Z',value:100},{at:'2026-10-28T10:00:01Z',value:200},{at:'bad',value:3},{at:'2026-10-28T10:00:03Z',value:NaN}];
 assert.deepEqual(graphSegments(points).map(a=>a.length),[1,1]);
});

test('recorder failures and input quality remain visible without live values', () => {
 const event={source:'live',health:'connecting',fuel:{status:'unavailable',quality:'UNAVAILABLE',quality_reasons:['iat_out_of_range'],factor:1,liters:1,distance_km:10},fuel_recording:{state:'failed',error:'disk full'}};
 const view=fuelView(event,0,'open');
 assert.equal(view.instant,'—');
 assert.match(view.note,/UNAVAILABLE/);
 assert.match(view.note,/запис/i);
});
