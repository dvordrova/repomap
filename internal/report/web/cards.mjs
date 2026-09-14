// Every card represents a saved report item. A bound input is displayed inside
// its explicit owner, without turning it into another architectural part.
export function groupInputs(inputs=[]) {
  const groups=new Map();
  for(const input of inputs){
    const key=input.activation||input.kind||'input';
    if(!groups.has(key))groups.set(key,{kind:key,title:input.groupLabel||input.kindLabel,inputs:[]});
    groups.get(key).inputs.push(input);
  }
  return [...groups.values()];
}
export function inputGroupsHeight(inputs=[],headingHeight=28){
  return groupInputs(inputs).reduce((height,group)=>height+headingHeight+group.inputs.reduce((h,input)=>h+input.height+8,0),0);
}
export function wrapText(text,width,font,measure){
    const lines=[''];
    for(const word of String(text||'').split(/\s+/).filter(Boolean)){
      const last=lines.length-1, candidate=lines[last]?`${lines[last]} ${word}`:word;
      if(measure(candidate,font)<=width){lines[last]=candidate;continue;}
      if(lines[last])lines.push('');
      for(const letter of word){const at=lines.length-1;if(lines[at]&&measure(lines[at]+letter,font)>width)lines.push(letter);else lines[at]+=letter;}
    }
    return lines;
}
export function prepareCards(records, inputOwner, measure, translate) {
  const kind=n=>translate(({request:'Request',command:'Command',interaction:'UI action',scheduled:'Scheduled task',continuous:'Background activity'})[n.activation]||n.kind||'Input');
  const groupKind=n=>translate(({request:'Incoming requests',command:'Commands',interaction:'User interactions',scheduled:'Scheduled tasks',continuous:'Background work'})[n.activation]||n.kind||'Inputs');
  const wrap=(text,width,font)=>wrapText(text,width,font,measure);
  const communicationChildren=new Set(records.filter(n=>n.branch==='communication').flatMap(n=>n.children||[]));
  const owners=new Map();
  for(const n of records){
    const owner=inputOwner[n.id];if(!owner)continue;
    const lines=wrap(n.title,202,'600 13px system-ui');
    const input={...n,displayTitle:lines.join('\n'),kindLabel:kind(n),groupLabel:groupKind(n),height:22+lines.length*18};
    if(!owners.has(owner))owners.set(owner,[]);
    owners.get(owner).push(input);
  }
  return records.filter(n=>!inputOwner[n.id]).map(n=>{
    const frame=!!n.children?.length;
    const title=wrap(n.title,228,'700 17px system-ui');
    const label=wrap(n.title,152,'12px system-ui');
    const inputs=owners.get(n.id)||[];
    const metadata=[n.language,n.componentKind?translate(n.componentKind):''].filter(Boolean).join(' · ');
    const roleLines=n.role?wrap(n.role,228,'600 13px system-ui'):[];
    const description=n.branch==='component'||n.category==='component'?n.summary:'';
    const descriptionLines=description?wrap(description,228,'13px system-ui'):[];
    const subtitle=n.category==='external'&&!n.title.endsWith(n.subtitle||'')?n.subtitle:'';
    const subtitleLines=subtitle?wrap(subtitle,228,'13px system-ui'):[];
    return {...n,name:n.title,title:title.join('\n'),labelTitle:label.join('\n'),inputs,metadata,role:roleLines.join('\n'),
      kindLabel:communicationChildren.has(n.id)?'':kind(n),description:descriptionLines.join('\n'),subtitle:subtitleLines.join('\n'),
      labelWidth:180,labelHeight:label.length*16,
      headerHeight:Math.max(64,32+title.length*22+(metadata?24:0)+(roleLines.length?8+roleLines.length*18:0)+(descriptionLines.length?12+descriptionLines.length*18:0)),
      width:260,height:frame?undefined:66+title.length*22+descriptionLines.length*18+
        (subtitleLines.length?8+subtitleLines.length*18:0)+inputGroupsHeight(inputs)};
  });
}
