// A part's code as the page data lists it (its reading, page_reading.go).
// It neither extends the analysis graph nor attaches relations to a member.
var repomapMembers = (function () {
  var inventories = new WeakMap();
  function sourceKey(source) { return source.Href || source.Open || (source.NoSource ? (source.Path ? JSON.stringify([source.Path,source.Line||0]) : source.Text) : ''); }
  // A tile's declaration by the same key: its link, else its place (a page
  // with no source link keys every declaration by its place, sourceKey).
  function symbolKey(symbol) { return symbol ? symbol.href || symbol.open || (symbol.path ? JSON.stringify([symbol.path,symbol.line||0]) : '') : ''; }
  // A declaration a reading lists by the same key: its link, else its own
  // key, else, with no source link, its place as written ("path:line").
  function declKey(decl) {
    if (!decl) return '';
    if (decl.href || decl.open || decl.key) return decl.href || decl.open || decl.key;
    var place = decl.no_source ? /^(.*):(\d+)$/.exec(decl.source || decl.at || '') : null;
    return place ? JSON.stringify([place[1], Number(place[2])]) : '';
  }
  function items(node) {
    if (inventories.has(node)) return inventories.get(node);
    var result = [], known = new Map();
    // A key type the model explained is listed first from its concept; its
    // row in Code in this part still says it is a key and carries its
    // fields, which the concept does not.
    function add(item) {
      var key = sourceKey(item.source);
      if (!key) return;
      var listed = known.get(key);
      if (listed) {
        if (item.key) listed.key = true;
        if (!listed.fields?.length && item.fields?.length) listed.fields = item.fields;
        return;
      }
      known.set(key, item); result.push(item);
    }
    (rmPage.data(node,'concepts')||[]).forEach(add);
    // Code in this part is every declaration of the part's reading (page
    // data), by kind and name as Go ordered them, a key marked, with the
    // model's line or alias where it wrote one; a type with its fields.
    var reading = typeof rmGroupReading === 'function' ? rmGroupReading(node) : null;
    if (reading) {
      var explained = new Map((rmPage.data(node,'explained')||[]).map(function (said) { return [sourceKey(said.source), said]; }));
      (reading.members||[]).forEach(function (kind) { (kind.decls||[]).forEach(function (index) {
        var decl = reading.decls[index], source = placeSource(decl.href, decl.open, decl.no_source, decl.at || decl.name), said = explained.get(sourceKey(source));
        add({name:decl.name, alias:said ? said.alias || '' : '', key:!!decl.bold, explanation:said ? said.explanation || '' : '', explanation_ref:said ? said.explanation_ref || '' : '', source:source,
          fields:(decl.fields||[]).map(function (field) { return {name:field.name, alias:'', explanation:'', source:placeSource(field.href, field.open, field.no_source, field.at || field.name)}; })});
      }); });
    }
    inventories.set(node, result); return result;
  }
  // A declaration's source as the reading column links it: its first line,
  // its editor action, or its place when it has neither.
  function placeSource(href, open, noSource, at) {
    var place = /^(.*):(\d+)$/.exec(at || '');
    return {Href:href || '', Open:open || '', NoSource:!!noSource, Path:place ? place[1] : '', Line:place ? Number(place[2]) : 0, Text:at || ''};
  }
  function size(node) {
    return {w:360,h:112 + Math.min(items(node).length,5)*66 + (items(node).length>5?32:0)};
  }
  function label(node,item) {
    return displayName(item)+(items(node).some(function(other){return other!==item&&other.name===item.name;})?' · '+item.source.Text:'');
  }
  function displayName(item){return item.alias&&item.alias!==item.name?item.alias+' · '+item.name:item.name;}
  function sourceLink(source) {
    var link=document.createElement(source.Href||source.Open?'a':'span');link.textContent=source.Text;
    if(source.Href){link.href=source.Href;link.target='_blank';}
    else if(source.Open){link.href='#';link.dataset.open=source.Open;}
    else if(source.NoSource)link.title=rmT('No source');
    return link;
  }
  // A part's complete list reads as the owner asked: the model's keys first,
  // then every other declaration by name, each a link into its code with no
  // line number (the file stands once in the heading). By file and line,
  // List commands' eighteen functions read as a column of numbers.
  function sorted(node){
    return items(node).slice().sort(function(a,b){return (b.key?1:0)-(a.key?1:0)||displayName(a).localeCompare(displayName(b),'en',{sensitivity:'base'})||displayName(a).localeCompare(displayName(b));});
  }
  // What a part is made of: how many declarations of each kind its list
  // holds (the kinds its tiles carry), and the files they are written in.
  function composition(node){
    var kinds={},counts={},files=[];
    try{(rmPage.data(node,'symbols')||[]).forEach(function(symbol){var key=symbolKey(symbol);if(key)kinds[key]=symbol.kind;});}catch(_){}
    items(node).forEach(function(item){
      var kind=kinds[sourceKey(item.source)]||'';
      var word=kind==='function'||kind==='method'?'{0} functions':kind==='type'?'{0} types':kind==='variable'?'{0} variables':'{0} declarations';
      counts[word]=(counts[word]||0)+1;
      var path=item.source.Path||'';if(path&&files.indexOf(path)<0)files.push(path);
    });
    return {total:items(node).length,counts:['{0} functions','{0} types','{0} variables','{0} declarations'].filter(function(word){return counts[word];}).map(function(word){return rmT(word,counts[word]);}).join(', '),files:files.sort()};
  }
  function grid(map,node,limit) {
    var grid=document.createElement('div');grid.className='map-member-grid';
    var list=limit===undefined?sorted(node):items(node).slice(0,limit);
    list.forEach(function(item){
      var row=document.createElement('div');row.className='map-member'+(item.key?' map-member-key':'');
      var name=sourceLink(item.source);name.className='map-member-name';name.textContent=displayName(item);
      name.dataset.memberSource=sourceKey(item.source);
      name.title=[item.explanation,item.source.Text,item.source.NoSource?rmT('No source'):''].filter(Boolean).join('\n');
      name.addEventListener('click',function(e){e.preventDefault();e.stopPropagation();map.showMember(node,item);});
      if(item.source.NoSource){name.setAttribute('tabindex','0');name.setAttribute('role','button');name.addEventListener('keydown',function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();map.showMember(node,item);}});}
      row.appendChild(name);
      if(item.fields?.length){
        var fields=document.createElement('div');fields.className='map-member-fields';
        item.fields.forEach(function(field,index){if(index)fields.appendChild(document.createTextNode(' · '));var link=sourceLink(field.source);link.textContent=field.name;fields.appendChild(link);});
        row.appendChild(fields);
      }
      grid.appendChild(row);
    });
    return grid;
  }
  function draw(map, node, box) {
    var svg = map.querySelector('svg'), previous = svg.querySelector('.map-member-layer');
    if (previous) previous.remove();
    if (!node || !box || !items(node).length) return;
    var layer = document.createElementNS('http://www.w3.org/2000/svg','foreignObject');
    layer.setAttribute('class','map-member-layer');
    layer.setAttribute('x',box.x + 16); layer.setAttribute('y',box.y + 70);
    layer.setAttribute('width',box.w - 32); layer.setAttribute('height',box.h - 78);
    var body = document.createElement('div'); body.className='map-member-content';
    var heading = document.createElement('div'); heading.className='map-member-label';
    heading.textContent=rmT('Code in this part')+' · '+items(node).length; body.appendChild(heading);
    body.appendChild(grid(map,node,5));
    if(items(node).length>5){
      var more=document.createElement('button');more.type='button';more.className='map-members-all';more.textContent=rmT('All {0} →',items(node).length);
      more.addEventListener('click',function(e){e.stopPropagation();map.showAllMembers(node);});body.appendChild(more);
    }
    layer.appendChild(body); svg.appendChild(layer);
  }
  return {sourceKey:sourceKey,symbolKey:symbolKey,declKey:declKey,items:items,sorted:sorted,composition:composition,size:size,draw:draw,grid:grid,label:label,displayName:displayName,sourceLink:sourceLink};
})();
