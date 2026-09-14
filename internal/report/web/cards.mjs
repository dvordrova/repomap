// Every card represents one saved report item, including each original input.
export function groupInputs(inputs=[],translate=text=>text) {
  const kinds=[['request','Incoming requests'],['command','Commands'],['background','Background work'],['interaction','User interactions'],['other','Other operations']];
  const groups=new Map(kinds.map(([kind,title])=>[kind,{kind,title:translate(title),inputs:[]}]));
  for(const input of inputs){
    const kind=['scheduled','continuous'].includes(input.activation)?'background':groups.has(input.activation)?input.activation:'other';
    groups.get(kind).inputs.push(input);
  }
  return [...groups.values()].filter(group=>group.inputs.length);
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

// A narrow external heading may need the full card width below its zoom mark.
// Measure the same heading for layout reservation and for the visible card.
export function overviewHeading(item,screenWidth,measure){
  const collection=['communication','inputs'].includes(item.branch);
  const font=collection?'700 13px system-ui':'700 18px system-ui';
  const lineHeight=collection?17:23;
  const title=item.name||item.title;
  const clearZoom=String(title).split(/\s+/).some(word=>measure(word,font)>screenWidth-64);
  const width=Math.max(1,Math.min(304,screenWidth-(clearZoom?(collection?16:32):64)));
  return {width,clearZoom,height:wrapText(title,width,font,measure).length*lineHeight+(clearZoom?32:0)};
}

export function prepareCards(records, _inputOwner, measure, translate) {
  const kind=n=>translate(({request:'Request',command:'Command',interaction:'UI action',scheduled:'Scheduled task',continuous:'Background activity'})[n.activation]||n.kind||'Input');
  const wrap=(text,width,font)=>wrapText(text,width,font,measure);
  const communicationChildren=new Set(records.filter(n=>n.branch==='communication').flatMap(n=>n.children||[]));
  const byID=new Map(records.map(n=>[n.id,n]));
  const areaNames=id=>(byID.get(id)?.children||[]).flatMap(child=>[
    ...(byID.get(child)?.branch==='area'?[byID.get(child).title]:[]),...areaNames(child),
  ]);
  return records.map(n=>{
    const frame=!!n.children?.length;
    const title=wrap(n.title,228,'700 17px system-ui');
    const label=wrap(n.title,152,'12px system-ui');
    const metadata=[n.language,n.componentKind?translate(n.componentKind):''].filter(Boolean).join(' · ');
    const roleLines=n.role?wrap(n.role,228,'600 13px system-ui'):[];
    const description=n.branch==='component'||n.category==='component'?n.summary:'';
    const descriptionLines=description?wrap(description,228,'13px system-ui'):[];
    const subtitle=n.category==='external'&&!n.title.endsWith(n.subtitle||'')?n.subtitle:'';
    const subtitleLines=subtitle?wrap(subtitle,228,'13px system-ui'):[];
    const names=n.branch==='component'?areaNames(n.id):[];
    // External headings may use the full width below the zoom mark. Its extra
    // row is reserved by overviewHeading, not by forcing a wider frame.
    const input=!!n.activation,collection=['communication','inputs'].includes(n.branch);
    const inputGroups=n.branch==='inputs'?groupInputs((n.children||[]).map(id=>byID.get(id)).filter(Boolean),translate):[];
    const widestWord=(text,font)=>Math.max(0,...String(text||'').split(/\s+/).map(word=>measure(word,font)));
    // Reserve complete physical pixels. A fractional fit round-off at the
    // longest-word boundary must not unexpectedly add a row below the zoom mark.
    const overviewMinWidth=Math.ceil(Math.max(widestWord(n.title,collection?'700 13px system-ui':'700 18px system-ui')+(collection?16:64),
      ...names.map(name=>widestWord(name,'500 13px system-ui')+32),
      ...inputGroups.map(group=>widestWord(group.title,'500 13px system-ui')+16)));
    // A short target name must not squeeze its area inventory into a column
    // of isolated words. Prefer two-line entries within the existing text
    // column; targets with short area names need no extra width.
    const twoLineWidth=text=>{
      const words=String(text).split(/\s+/).filter(Boolean);
      if(!words.length)return 0;
      return Math.min(...words.map((_,i)=>Math.max(
        measure(words.slice(0,i+1).join(' '),'500 13px system-ui'),
        measure(words.slice(i+1).join(' '),'500 13px system-ui'))));
    };
    const overviewPreferredWidth=Math.ceil(Math.max(overviewMinWidth,
      ...names.map(name=>32+Math.min(304,twoLineWidth(name)))));
    const overviewHeightAtWidth=['component','communication','inputs'].includes(n.branch)?(width,{availableHeight=Infinity}={})=>{
      const heading=overviewHeading(n,width,measure);
      if(n.branch==='inputs')return Math.max(44,16+Math.max(32,heading.height)+inputGroups.reduce((h,group)=>h+6+wrap(group.title,Math.max(1,Math.min(304,width-16)),'500 13px system-ui').length*18,0));
      // The inventory remains complete in the scrollable summary. Its first
      // entrance sets the minimum usable height; fitting the entire list would
      // enlarge the world and make its fitted text smaller again.
      const rows=names.map(name=>10+wrap(name,Math.max(1,Math.min(304,width-32)),'500 13px system-ui').length*18);
      const base=(n.branch==='communication'?16:32)+heading.height;
      const list=rows.length?17+rows.reduce((sum,row)=>sum+row,0):0;
      const controlHeight=28+(n.branch==='communication'?16:32);
      return Math.max(controlHeight,base+(base+list>availableHeight&&rows.length?17+rows[0]:list));
    }:undefined;
    return {...n,category:input?'input':n.category,name:n.title,title:title.join('\n'),labelTitle:label.join('\n'),inputGroups,metadata,role:roleLines.join('\n'),overviewHeightAtWidth,overviewMinWidth,overviewPreferredWidth,
      roleLabel:!input&&['core','triggers'].includes(n.lane)?translate(n.lane==='core'?'Core':'Entrypoints'):'',
      kindLabel:communicationChildren.has(n.id)||!input&&['core','triggers'].includes(n.lane)?'':kind(n),description:descriptionLines.join('\n'),subtitle:subtitleLines.join('\n'),
      labelWidth:180,labelHeight:label.length*16,
      headerHeight:Math.max(64,32+title.length*22+(metadata?24:0)+(roleLines.length?8+roleLines.length*18:0)+(descriptionLines.length?12+descriptionLines.length*18:0)),
      width:260,height:frame?undefined:66+title.length*22+descriptionLines.length*18+
        (subtitleLines.length?8+subtitleLines.length*18:0)};
  });
}
