import test from 'node:test';
import assert from 'node:assert/strict';
import { fuelView } from './fuel.mjs';
const now='2026-10-28T12:00:00Z';
const event={ source:'live',health:'connected',server_time:now,
 fuel:{status:'available',flow_lph:2,instant_l100:5,speed_kmh:40,low_speed_kmh:5,
 flow_until:'2026-10-28T12:00:03Z',speed_until:'2026-10-28T12:00:01Z',liters:1,distance_km:20,covered_km:10,average_l100:10,
 method:'speed_density',factor:1,incomplete:true,coverage_reasons:['unavailable_input'],persistence:'saved',settings:{unverified_iat:true}} };
test('fuel switches units and retains partial historical totals',()=>{
 const v=fuelView(event,0,'open');assert.equal(v.instant,'5.0');assert.equal(v.unit,'л/100 км');assert.equal(v.liters,'1.00');assert.match(v.note,/частичн/i);assert.match(v.note,/IAT/);
 const idle=fuelView({...event,fuel:{...event.fuel,speed_kmh:0,instant_l100:null}},0,'open');assert.equal(idle.instant,'2.0');assert.equal(idle.unit,'л/ч');
});
test('speed expiry switches to flow and flow expiry hides only instantaneous values',()=>{
 const v=fuelView(event,1000,'open');assert.equal(v.instant,'2.0');assert.equal(v.unit,'л/ч');assert.match(v.note,/Скорость/);
 const stale=fuelView(event,3000,'open');assert.equal(stale.instant,'—');assert.equal(stale.liters,'1.00');
 const disconnected=fuelView(event,0,'retrying');assert.equal(disconnected.instant,'—');assert.equal(disconnected.average,'10.0');
});
test('missing or nonfinite values never become zero or calibrated truth',()=>{
 assert.equal(fuelView(null,0,'connecting').instant,'—');
 const v=fuelView({...event,fuel:{...event.fuel,flow_lph:NaN,average_l100:null,persistence:'persistence_error'}},0,'open');
 assert.equal(v.instant,'—');assert.equal(v.average,'—');assert.match(v.note,/сохран/i);assert.match(v.model,/Оценка/);
 const unavailable=fuelView({...event,fuel:{...event.fuel,status:'unavailable',flow_lph:null,reason:'unverified_iat'}},0,'open');
 assert.equal(unavailable.instant,'—');assert.match(unavailable.note,/IAT/);
});

test('local mutations send revisioned JSON and retain server errors', async()=>{
 const {fuelRequest}=await import('./fuel-controls.mjs');
 let called;
 const response=await fuelRequest('/api/fuel/reset',{expected_revision:7},async(path,options)=>{
  called={path,options};return {ok:true,json:async()=>({revision:8})};
 });
 assert.equal(response.revision,8);assert.equal(called.options.headers['Content-Type'],'application/json');assert.equal(JSON.parse(called.options.body).expected_revision,7);
 await assert.rejects(fuelRequest('/api/fuel/reset',{},async()=>({ok:false,text:async()=> 'revision changed'})),/revision changed/);
});

test('unknown source does not acquire a simulator label',()=>{
 const v=fuelView({...event,source:'unknown'},0,'open');assert.match(v.model,/UNKNOWN/);assert.equal(v.instant,'—');
});
